package harvest

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/sfomuseum/go-blobcache"
	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest"
	"github.com/sfomuseum/go-embeddingsdb/parquet"
)

func Run(ctx context.Context) error {
	fs := DefaultFlagSet()
	return RunWithFlagSet(ctx, fs)
}

func RunWithFlagSet(ctx context.Context, fs *flag.FlagSet) error {

	opts, err := RunOptionsFromFlagSet(fs)

	if err != nil {
		return err
	}

	return RunWithOptions(ctx, opts)
}

func RunWithOptions(ctx context.Context, opts *RunOptions) error {

	if opts.Verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Verbose logging enabled")
	}

	if len(opts.Models) == 0 {
		slog.Warn("No models defined")
	}

	slog.Debug("Set up embeddings client", "uri", opts.EmbeddingsClientURI)

	emb_cl, err := embeddings.NewEmbedder32(ctx, opts.EmbeddingsClientURI)

	if err != nil {
		return fmt.Errorf("Failed to create embeddings client, %w", err)
	}

	slog.Debug("Set up blobcache", "uri", opts.CacheURI)

	blob_c, err := blobcache.NewBlobCache(ctx, opts.CacheURI)

	if err != nil {
		return fmt.Errorf("Failed to create blob cache, %w", err)
	}

	defer blob_c.Close()

	http_cl := http.NewClient()

	cache_opts := &http.GetWithCacheOptions{
		CheckLastModTime: opts.CacheCheckLastmodified,
		Client:           http_cl,
		BlobCache:        blob_c,
	}

	if precache {
		slog.Warn("-precache flag set, assigning parquet output to /dev/null")
		opts.Output = "/dev/null"
	}

	slog.Debug("Set up writer", "output", opts.Output)

	wr, err := parquet.NewWriter(ctx, opts.Output)

	if err != nil {
		return fmt.Errorf("Failed to create new writer, %w", err)
	}

	slog.Debug("Set up harvester", "uri", opts.HarvesterURI)

	harvester, err := harvest.NewHarvester(ctx, opts.HarvesterURI)

	if err != nil {
		return fmt.Errorf("Failed to create harvester, %w", err)
	}

	count := int64(0)
	done_ch := make(chan bool)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	t1 := time.Now()

	go func() {
		for {
			select {
			case <-done_ch:
				return
			case <-ticker.C:
				slog.Info("Harvested rows", "count", atomic.LoadInt64(&count), "time", time.Since(t1))
			}
		}
	}()

	throttle := make(chan bool, opts.Workers)

	for i := 0; i < opts.Workers; i++ {
		throttle <- true
	}

	iterate_opts := &harvest.IterateOptions{
		EmbeddingsClient: emb_cl,
		CacheOptions:     cache_opts,
		Throttle:         throttle,
		Models:           opts.Models,
		PreCache:         opts.Precache,
		Verbose:          opts.Verbose,
		Params:           opts.Params,
	}

	slog.Debug("Harvest records")

	for records, err := range harvester.Iterate(ctx, iterate_opts) {

		if err != nil {
			return fmt.Errorf("Objects iterator yielded an error, %w", err)
		}

		count_records := len(records)

		if count_records == 0 {
			continue
		}

		atomic.AddInt64(&count, int64(count_records))

		_, err = wr.Write(records)

		if err != nil {
			return fmt.Errorf("Failed to write records, %w", err)
		}

		wr.Flush()
	}

	err = wr.Close()

	if err != nil {
		return fmt.Errorf("Failed to close after writing, %w", err)
	}

	slog.Info("Harvesting complete", "total", atomic.LoadInt64(&count), "time", time.Since(t1))

	done_ch <- true
	return nil
}
