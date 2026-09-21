package parquet

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sfomuseum/go-embeddingsdb/client"
)

type ImportOptions struct {
	Client  client.Client
	Start   int64
	End     int64
	Workers int
}

// Import [*embeddingsdb.Record] records stored in one or more Parquet files identified by 'uris' and add them to an embeddings database using 'cl'.
func Import(ctx context.Context, cl client.Client, uris ...string) (int64, error) {

	opts := &ImportOptions{
		Client:  cl,
		Start:   int64(0),
		End:     int64(0),
		Workers: 1,
	}

	return ImportWithOptions(ctx, opts, uris...)
}

// ImportWithOptions [*embeddingsdb.Record] records whose position is between 'start' and 'end' (inclusive) stored in one or more Parquet files identified by 'uris' and add them to an embeddings database using 'cl'.
func ImportWithOptions(ctx context.Context, opts *ImportOptions, uris ...string) (int64, error) {

	logger := slog.Default()

	current := ""
	count := int64(0)
	total := int64(0)

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	go func() {

		for {
			select {
			case <-ticker.C:
				slog.Info("Import stats", "current", current, "count", count, "total", total)
			}
		}

	}()

	seen := new(sync.Map)
	wg := new(sync.WaitGroup)

	throttle := make(chan bool, opts.Workers)

	for i := 0; i < opts.Workers; i++ {
		throttle <- true
	}

	err_ch := make(chan error)

	for _, uri := range uris {

		current = uri
		count = 0

		logger := slog.Default()
		logger = logger.With("uri", uri)

		for rec, err := range Iterate(ctx, uri) {

			if err != nil {
				logger.Error("Iterator yielded an error", "error", err)
				return total, err
			}

			count += 1
			total += 1

			if opts.Start > 0 && opts.Start > count {
				continue
			}

			select {
			case <-ctx.Done():
				logger.Info("Context signaled done, exiting")
				return total, nil
			case err := <-err_ch:
				return total, fmt.Errorf("Failed to add record '%s', %w", rec.Key(), err)
			default:

				<-throttle

				wg.Go(func() {

					defer func() {
						throttle <- true
					}()

					key := rec.Key()
					_, ok := seen.LoadOrStore(key, true)

					if ok {
						logger.Warn("Record already indexed, skipping", "key", key)
						return
					}

					err := opts.Client.AddRecord(ctx, rec)

					if err != nil {
						logger.Error("Failed to add record", "key", rec.Key(), "error", err)
						err_ch <- err
						return
					}

					logger.Debug("Add record", "key", rec.Key(), "count", count, "total", total)
				})
			}

			if opts.End > 0 && opts.End >= count {
				break
			}
		}

		logger.Debug("Finished iterating uri", "count", count, "total", total)
	}

	wg.Wait()

	logger.Debug("Finished importing all", "total", total)
	return total, nil
}
