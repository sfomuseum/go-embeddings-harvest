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
	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest"
	"github.com/sfomuseum/go-embeddingsdb/parquet"
	"github.com/sfomuseum/go-flags/flagset"
)

func Run(ctx context.Context) error {
	fs := DefaultFlagSet()
	return RunWithFlagSet(ctx, fs)
}

func RunWithFlagSet(ctx context.Context, fs *flag.FlagSet) error {

	flagset.Parse(fs)

	if verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Verbose logging enabled")
	}

	if len(models) == 0 {
		slog.Warn("No models defined")
	}

	slog.Debug("Set up embeddings client", "uri", embeddings_client_uri)

	emb_cl, err := sfom_embeddings.NewEmbedder32(ctx, embeddings_client_uri)

	if err != nil {
		return fmt.Errorf("Failed to create embeddings client, %w", err)
	}

	slog.Debug("Set up blobcache", "uri", cache_uri)

	blob_c, err := blobcache.NewBlobCache(ctx, cache_uri)

	if err != nil {
		return fmt.Errorf("Failed to create blob cache, %w", err)
	}

	defer blob_c.Close()

	http_cl := http.NewClient()

	cache_opts := &http.GetWithCacheOptions{
		CheckLastModTime: cache_check_lastmod,
		Client:           http_cl,
		BlobCache:        blob_c,
	}

	if precache {
		slog.Warn("-precache flag set, assigning parquet output to /dev/null")
		output = "/dev/null"
	}

	slog.Debug("Set up writer", "output", output)

	wr, err := parquet.NewWriter(ctx, output)

	if err != nil {
		return fmt.Errorf("Failed to create new writer, %w", err)
	}

	slog.Debug("Set up harvester", "uri", harvester_uri)

	harvester, err := harvest.NewHarvester(ctx, harvester_uri)

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

	throttle := make(chan bool, workers)

	for i := 0; i < workers; i++ {
		throttle <- true
	}

	iterate_opts := &harvest.IterateOptions{
		EmbeddingsClient: emb_cl,
		CacheOptions:     cache_opts,
		Throttle:         throttle,
		Models:           models,
		PreCache:         precache,
		Verbose:          verbose,
		Params:           params,
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
