package harvest

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"iter"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/aaronland/go-jsonl/walk"
	_ "github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-embeddingsdb"
)

func init() {
	MustRegisterHarvester(context.Background(), "null", NewJSONLHarvester)
}

type JSONLHarvester struct {
	Harvester
	path string
}

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

func (h *JSONLHarvester) Iterate(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		r, err := os.Open(h.path)

		if err != nil {
			yield(nil, err)
			return
		}

		defer r.Close()

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		buffer := NewBuffer[*embeddingsdb.Record](100)
		defer buffer.Close()

		if opts.Verbose {
			buffer.StartStatsTicker()
		}

		wg := new(sync.WaitGroup)

		for walk_rec, err := range walk.IterateReader(ctx, r) {

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

			row := maps.Clone(walk_rec)

			wg.Go(func() {

				defer func() {
					opts.Throttle <- true
				}()

				var rec *embeddingsdb.Record

				err = json.Unmarshal(row.Body, &rec)

				if err != nil {
					yield(nil, err)
					return
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

func (h *JSONLHarvester) Close() error {
	return nil
}
