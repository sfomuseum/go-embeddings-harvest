package harvest

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"sync"

	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-csvdict/v2"
	"github.com/sfomuseum/go-embeddingsdb"
)

func init() {
	MustRegisterHarvester(context.Background(), "moma", NewMuseumOfModernArtHarvester)
}

type MuseumOfModernArtHarvester struct {
	Harvester
	path_objects string
}

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

		records_ch := make(chan []*embeddingsdb.Record)
		err_ch := make(chan error)

		go func() {

			for {
				select {
				case <-ctx.Done():
					return
				case err := <-err_ch:

					if !yield(nil, err) {
						cancel()
						return
					}

				case records := <-records_ch:

					if !yield(records, nil) {
						cancel()
						return
					}
				}
			}
		}()

		wg := new(sync.WaitGroup)

		for row, err := range objects_r.Iterate() {

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
					err_ch <- err
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
					err_ch <- err
					return
				}

				if opts.PreCache {
					records_ch <- make([]*embeddingsdb.Record, 0)
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
					err_ch <- err
					return
				}

				records_ch <- records
				logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
			})
		}

		wg.Wait()
	}
}

func (h *MuseumOfModernArtHarvester) Close() error {
	return nil
}
