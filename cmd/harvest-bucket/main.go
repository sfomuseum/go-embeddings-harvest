package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/aaronland/gocloud/blob/walk"
	"github.com/aaronland/gocloud/blob/bucket"	
	"github.com/sfomuseum/go-blobcache"
	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest"
	"github.com/sfomuseum/go-embeddingsdb/parquet"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
)

func main() {

	var embeddings_client_uri string
	var cache_uri string

	var bucket_uri string

	var workers int
	var output string
	var verbose bool
	var models multi.MultiCSVString

	fs := flagset.NewFlagSet("nga")

	fs.IntVar(&workers, "workers", 5, "The number of workers to use to fetch images (and derive embeddings) concurrently")
	fs.Var(&models, "model", "One or more models to derive embeddings for. This may also be a comma-separated list.")

	fs.StringVar(&bucket_uri, "bucket-uri", "", "...")
	
	fs.StringVar(&cache_uri, "cache-uri", "null://", "A register gocloud.dev/blob.Bucket URI to use for caching images. If null:// then no images will be cached.")

	fs.StringVar(&output, "output", "-", "The path where Parquet-encoded data should be written. If \"-\" then data will be written to STDOUT.")
	fs.StringVar(&embeddings_client_uri, "embeddings-client-uri", "mobileclip://?client-uri=grpc://localhost:8080", "A registered sfomuseum/go-embeddingsdb/client.Client URI.")
	fs.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging.")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Generate Parquet-encoded embeddings from the Museum of Modern Art (MoMA) open data release.\n")
		fmt.Fprintf(os.Stderr, "Usage:\n\t%s [options]", os.Args[0])
		fmt.Fprintf(os.Stderr, "Valid options are:\n")
		fs.PrintDefaults()
	}

	flagset.Parse(fs)

	if verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Verbose logging enabled")
	}

	ctx := context.Background()

	if len(models) == 0 {
		log.Fatal("No models defined")
	}

	emb_cl, err := sfom_embeddings.NewEmbedder32(ctx, embeddings_client_uri)

	if err != nil {
		log.Fatalf("Failed to create embeddings client, %v", err)
	}

	blob_c, err := blobcache.NewBlobCache(ctx, cache_uri)

	if err != nil {
		log.Fatalf("Failed to create blob cache, %v", err)
	}

	defer blob_c.Close()

	wr, err := parquet.NewWriter(ctx, output)

	if err != nil {
		log.Fatalf("Failed to create new writer, %v", err)
	}

	b, err := bucket.OpenBucket(ctx, bucket_uri)

	if err != nil {
		log.Fatalf("Failed to open bucket, %v", err)
	}

	count := int64(0)
	done_ch := make(chan bool)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-done_ch:
				return
			case <-ticker.C:
				slog.Info("Processed rows", "count", count)
			}
		}
	}()

	throttle := make(chan bool, workers)

	for i := 0; i < workers; i++ {
		throttle <- true
	}

	wg := new(sync.WaitGroup)

	for obj, err := range walk.WalkBucketWithIter(ctx, b, ".") {

		if err != nil {
			log.Fatalf("Bucket iterator yielded an error, %v", err)
		}

		// Check file format here...
		
		<-throttle

		wg.Go(func() {

			defer func() {
				throttle <- true
			}()

			logger := slog.Default()
			logger = logger.With("key", obj.Key)
			
			count += 1

			depiction_id := ""
			subject_id := ""

			im_r, err := b.NewReader(ctx, obj.Key, nil)

			if err != nil {
				logger.Error("Failed to open object for reading", "error", err)
				return
			}
			
			im_body, err := io.ReadAll(im_r)
			im_r.Close()

			if err != nil {
				logger.Error("Failed to read object", "error", err)
				return
			}
			
			attrs := map[string]string{
				"type":               "image",
				"preview":            "",
				"subject_url":        "",
				"subject_title":      "",
				"subject_creditline": "",
				"provider_name":      "",
				"provider_url":       "",
			}

			derive_opts := &harvest.DeriveEmbeddingsRecordsOptions{
				Provider:    "",
				DepictionId: depiction_id,
				SubjectId:   subject_id,
				Attributes:  attrs,
				Models:      models,
				Body:        im_body,
			}

			records, err := harvest.DeriveEmbeddingsRecords(ctx, emb_cl, derive_opts)

			if err != nil {
				logger.Error("Failed to derive embeddings records", "error", err)
				return
			}

			if len(records) == 0 {
				logger.Warn("Failed to derive embeddings")
				return
			}

			_, err = wr.Write(records)

			if err != nil {
				logger.Error("Failed to write records buffer", "error", err)
				return
			}

			logger.Debug("Wrote embeddings for exhibition image")
		})

		wr.Flush()
	}

	wg.Wait()

	err = wr.Close()

	if err != nil {
		log.Fatalf("Failed to close after writing, %v", err)
	}

}
