package harvest

// https://www.clevelandart.org/open-access
// https://openaccess-api.clevelandart.org/#fields
// https://github.com/ClevelandMuseumArt/openaccess

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-csvdict/v2"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/tidwall/gjson"
)

func init() {
	MustRegisterHarvester(context.Background(), "cma", NewClevelandMuseumArtHarvester)
}

// ClevelandMuseumArtHarvester implements the Harvester interface to parse and derive
// vector embeddings from the Cleveland Museum of Art's OpenAccess dataset.
type ClevelandMuseumArtHarvester struct {
	Harvester
	path_objects string
}

// NewClevelandMuseumArtHarvester instantiates and returns a new Harvester for the
// Cleveland Museum of Art dataset. It expects a URI formatted with the "cma" scheme
// containing the absolute path to the data.csv source file.
//
// Example:
//
//	cma:///usr/local/data/cma/openaccess/data.csv
func NewClevelandMuseumArtHarvester(ctx context.Context, uri string) (Harvester, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	h := &ClevelandMuseumArtHarvester{
		path_objects: u.Path,
	}

	return h, nil
}

// Iterate parses the underlying CMA collection CSV row-by-row, resolving the primary
// web image and any associated alternate images. It yields chunks of mapped embeddingsdb.Record
// datasets through a functional iterator sequence.
func (h *ClevelandMuseumArtHarvester) Iterate(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

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

		// This is what we use to process records concurrently, capturing and
		// yielding records without spilling outside of the main loop which causes
		// all kinds of iterator/yield pain.
		
		buffer := NewBuffer[*embeddingsdb.Record](1000, yield)
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

			<-opts.Throttle

			wg.Go(func() {

				defer func() {
					opts.Throttle <- true
				}()

				if row["image_web"] == "" {
					return
				}

				logger := slog.Default()
				logger = logger.With("object", row["accession_number"])

				images := []string{
					row["image_web"],
				}

				if row["alternate_images"] != "" {

					alt_str := strings.ReplaceAll(row["alternate_images"], "'", "\"")
					rsp := gjson.Get(alt_str, "#.web.url")

					for _, im := range rsp.Array() {
						images = append(images, im.String())
					}
				}

				logger.Debug("Process images for object", "count", len(images))

				for _, im_url := range images {

					fname := filepath.Base(im_url)
					ext := filepath.Ext(fname)

					depiction_id := strings.Replace(fname, ext, "", 1)
					subject_id := row["accession_number"]

					logger := slog.Default()
					logger = logger.With("subject", subject_id)
					logger = logger.With("depiction", depiction_id)

					logger.Debug("Fetch image", "url", im_url)

					im_body, err := http.GetBytesWithCacheAndOptions(ctx, opts.CacheOptions, im_url)

					if err != nil {
						logger.Error("Failed to retrieve image", "url", im_url, "error", err)
						continue
					}

					if opts.PreCache {
						continue
					}

					attrs := map[string]string{
						"type":               "image",
						"preview":            im_url,
						"subject_url":        row["url"],
						"subject_title":      row["title"],
						"subject_creditline": row["tombstone"],
						"provider_name":      "Cleveland Museum of Art",
						"provider_url":       "https://clevelandart.org/",
					}

					derive_opts := &DeriveEmbeddingsRecordsOptions{
						Provider:    "cma",
						DepictionId: depiction_id,
						SubjectId:   subject_id,
						Attributes:  attrs,
						Models:      opts.Models,
						Body:        im_body,
					}

					records, err := DeriveEmbeddingsRecords(ctx, opts.EmbeddingsClient, derive_opts)

					if err != nil {
						logger.Error("Failed to derive embeddings", "error", err)
						continue
					}

					if len(records) > 0 {

						if !buffer.Append(records...) {
							logger.Error("Appending and flushing records returned false")
							return
						}
					}
				}

			})

			if !buffer.Flush() {
				slog.Error("Flushing remaining records returned false")
				return
			}
		}

		wg.Wait()
	}
}

// Close gracefully closes down internal states managed by the ClevelandMuseumArtHarvester.
// It matches the Harvester cleanup interface requirement and currently behaves as a no-op.
func (h *ClevelandMuseumArtHarvester) Close() error {
	return nil
}
