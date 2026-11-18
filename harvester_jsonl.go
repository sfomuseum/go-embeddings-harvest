package harvest

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/aaronland/go-jsonl/walk"
	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/oembeddings"
)

func init() {
	MustRegisterHarvester(context.Background(), "jsonl", NewJSONLHarvester)
}

// JSONLHarvester implements the Harvester interface to parse data and
// derive vector embeddings from a line-separated file containing JSON-encoded
// [embeddingsdb.Record] records. These records are expected to have empty 'model'
// and 'embeddings' properties.
type JSONLHarvester struct {
	Harvester
	path string
}

// NewJSONLHarvester instantiates and returns a new Harvester configured for
// processing files containing line-separated JSON-encoded [embeddingsdb.Record] records.
// It requires a scheme-prefixed URI mapping the absolute file location of the JSONL file.
//
// Example:
//
//	json:///usr/local/data/collection.jsonl
func NewJSONLHarvester(ctx context.Context, uri string) (Harvester, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to parse URI, %w")
	}

	if u.Path == "" {
		return nil, fmt.Errorf("Missing path to harvest")
	}

	abs_path, err := filepath.Abs(u.Path)

	if err != nil {
		return nil, fmt.Errorf("Failed to derive absolute path, %w", err)
	}

	info, err := os.Stat(abs_path)

	if err != nil {
		return nil, fmt.Errorf("Failed to stat path to harvest, %w", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("Path is a directory")
	}

	h := &JSONLHarvester{
		path: abs_path,
	}

	return h, nil
}

// Iterate processes [embeddingsdb.Record] records sequentially by interpreting rows,
// pulling out image records via row attributes, and yielding mapped database records
// through an asynchronous background iterator.
func (h *JSONLHarvester) Iterate(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		r, err := os.Open(h.path)

		if err != nil {
			yield(nil, err)
			return
		}

		defer r.Close()

		walk_opts := &walk.IterateOptions{
			ValidateJSON: false,
			FormatJSON:   false,
			QuerySet:     nil,
			IsBzip:       false,
			Filter:       nil,
		}

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		buffer := NewBuffer[*embeddingsdb.Record](100)
		defer buffer.Close()

		if opts.Verbose {
			buffer.StartStatsTicker()
		}

		wg := new(sync.WaitGroup)

		for walk_rec, err := range walk.IterateReader(ctx, walk_opts, r) {

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

				var rec *embeddingsdb.Record

				err = json.Unmarshal(walk_rec.Body, &rec)

				if err != nil {
					slog.Warn("Failed to unmarshal record", "error", err)
					return
				}

				logger := slog.Default()
				logger = logger.With("subject", rec.SubjectId)

				oe, err := oembeddings.FromAttributes(rec.Attributes)

				if err != nil {
					logger.Warn("Failed to derive oembeddings from attributes", "error", err)
					return
				}

				var derive_opts *DeriveEmbeddingsRecordsOptions

				switch oe.Type {
				case "image":

					im_url := oe.Preview

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

					logger.Debug("Fetch image", "url", im_url)

					im_body, err := http.GetBytesWithCacheAndOptions(ctx, opts.CacheOptions, im_url)

					if err != nil {
						logger.Error("Failed to retrieve image", "url", im_url, "error", err)
						return
					}

					if opts.PreCache {
						return
					}

					derive_opts = &DeriveEmbeddingsRecordsOptions{
						Provider:    rec.Provider,
						DepictionId: rec.DepictionId,
						SubjectId:   rec.SubjectId,
						Attributes:  oe.AsMap(),
						Models:      opts.Models,
						Body:        im_body,
					}

					logger.Debug("Wrote embeddings for exhibition image", "url", im_url)

				default:
					logger.Warn("Unsupported record type", "type", oe.Type)
					return
				}

				records, err := DeriveEmbeddingsRecords(ctx, opts.EmbeddingsClient, derive_opts)

				if err != nil {
					logger.Error("Failed to derive embeddings records", "error", err)
					return
				}

				if len(records) > 0 {
					buffer.Append(records...)
				}

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

// Close releases active resource pipelines managed by the JSONLHarvester.
// It fulfills the abstract Harvester interface and functions as a standard no-op.
func (h *JSONLHarvester) Close() error {
	return nil
}
