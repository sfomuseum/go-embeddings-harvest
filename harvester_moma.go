package harvest

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"maps"
	"net/url"
	"sync"

	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-csvdict/v2"
	"github.com/sfomuseum/go-embeddingsdb"
)

func init() {
	MustRegisterHarvester(context.Background(), "moma", NewMuseumOfModernArtHarvester)
}

// MuseumOfModernArtHarvester implements the Harvester interface to parse data and
// derive vector embeddings from the Museum of Modern Art's open collection release.
type MuseumOfModernArtHarvester struct {
	Harvester
	path_objects string
}

// NewMuseumOfModernArtHarvester instantiates and returns a new Harvester configured for
// MoMA collection processing. It requires a scheme-prefixed URI mapping the absolute file
// location of MoMA's openaccess Artworks.csv dataset.
//
// Example:
//
//	moma:///usr/local/data/moma/collection/Artworks.csv
func NewMuseumOfModernArtHarvester(ctx context.Context, uri string) (Harvester, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	h := &MuseumOfModernArtHarvester{
		path_objects: u.Path,
	}

	return h, nil
}

// Iterate processes MoMA collection artwork objects sequentially by interpreting rows,
// pulling out image records via target SHA signature keys, and yielding mapped
// database records through an asynchronous background iterator.
func (h *MuseumOfModernArtHarvester) Iterate(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		// TBD: Iterate through objects_r twice, first to calculate total count
		// and second to process actual records. This would allow for a more-better
		// progress meter...

		objects_r, err := csvdict.NewReaderFromPath(h.path_objects)

		if err != nil {
			yield(nil, fmt.Errorf("Failed to create CSV reader for CMA data, %w", err))
			return
		}

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		buffer := NewBuffer[*embeddingsdb.Record](100)
		defer buffer.Close()

		if opts.Verbose {
			buffer.StartStatsTicker()
		}

		wg := new(sync.WaitGroup)

		for iter_row, err := range objects_r.Iterate() {

			select {
			case <-ctx.Done():
				return
			default:
				if err != nil {
					yield(nil, err)
					return
				}
			}

			if err != nil {
				yield(nil, fmt.Errorf("Artworks iterator yielded an error, %w", err))
				return
			}

			<-opts.Throttle

			row := maps.Clone(iter_row)

			wg.Go(func() {

				defer func() {
					opts.Throttle <- true
				}()

				depiction_id := ""
				subject_id := row["ObjectID"]
				im_url := row["ImageURL"]

				logger := slog.Default()
				logger = logger.With("subject", subject_id)

				if im_url == "" {
					logger.Warn("No image URL")
					return
				}

				im_u, err := url.Parse(im_url)

				if err != nil {
					logger.Error("Failed to parse image URL", "url", im_url, "error", err)
					return
				}

				im_q := im_u.Query()

				if !im_q.Has("sha") {
					logger.Warn("Image URL missing ?sha", "url", im_url)
					return
				}

				depiction_id = im_q.Get("sha")

				logger.Debug("Fetch image", "url", im_url)

				im_body, err := http.GetBytesWithCacheAndOptions(ctx, opts.CacheOptions, im_url)

				if err != nil {
					logger.Error("Failed to retrieve image", "url", im_url, "error", err)
					return
				}

				if opts.PreCache {
					return
				}

				attrs := map[string]string{
					"type":               "image",
					"preview":            im_url,
					"subject_url":        row["URL"],
					"subject_title":      row["Title"],
					"subject_creditline": row["CreditLine"],
					"provider_name":      "Museum of Modern Art",
					"provider_url":       "https://www.moma.org/",
				}

				derive_opts := &DeriveEmbeddingsRecordsOptions{
					Provider:    "moma",
					DepictionId: depiction_id,
					SubjectId:   subject_id,
					Attributes:  attrs,
					Models:      opts.Models,
					Body:        im_body,
				}

				records, err := DeriveEmbeddingsRecords(ctx, opts.EmbeddingsClient, derive_opts)

				if err != nil {
					logger.Error("Failed to derive embeddings records", "error", err)
					return
				}

				if len(records) > 0 {
					buffer.Append(records...)
				}

				logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
			})

			if records := buffer.CollectAndReset(false); records != nil {
				if !yield(records, nil) {
					cancel()
					return
				}
			}
		}

		wg.Wait()

		if records := buffer.CollectAndReset(true); records != nil {
			if !yield(records, nil) {
				cancel()
				return
			}
		}
	}
}

// Close releases active resource pipelines managed by the MuseumOfModernArtHarvester.
// It fulfills the abstract Harvester interface and functions as a standard no-op.
func (h *MuseumOfModernArtHarvester) Close() error {
	return nil
}
