package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/sfomuseum/go-blobcache"
	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-csvdict/v2"
	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest"
	harvest_http "github.com/sfomuseum/go-embeddings-harvest/http"
	"github.com/sfomuseum/go-embeddingsdb/parquet"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
)

func main() {

	var embeddings_client_uri string
	var cache_uri string
	var cache_check_lastmod bool

	var artworks string

	var workers int
	var output string
	var verbose bool
	var models multi.MultiCSVString

	fs := flagset.NewFlagSet("nga")

	fs.StringVar(&artworks, "artworks", "", "The path to the 'Artworks.csv' file contained in the MuseumofModernArt/collection GitHub repository.")

	fs.IntVar(&workers, "workers", 5, "The number of workers to use to fetch images (and derive embeddings) concurrently")
	fs.Var(&models, "model", "One or more models to derive embeddings for. This may also be a comma-separated list.")

	fs.StringVar(&cache_uri, "cache-uri", "null://", "A register gocloud.dev/blob.Bucket URI to use for caching images. If null:// then no images will be cached.")
	fs.BoolVar(&cache_check_lastmod, "cache-check-lastmod", true, "A boolean value to indicate whether the last modified date of an object to harvest should be compared against the local cache.")

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

	artworks_r, err := csvdict.NewReaderFromPath(artworks)

	if err != nil {
		log.Fatalf("Failed to create CSV reader for artworks, %v", err)
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

	for row, err := range artworks_r.Iterate() {

		if err != nil {
			log.Fatalf("Artworks iterator yielded an error, %v", err)
		}

		<-throttle

		wg.Go(func() {

			defer func() {
				throttle <- true
			}()

			count += 1

			depiction_id := ""
			subject_id := row["ObjectID"]
			im_url := row["ImageURL"]

			logger := slog.Default()
			logger = logger.With("subject", subject_id)

			if im_url == "" {
				logger.Error("No image URL")
				return
			}

			im_u, err := url.Parse(im_url)

			if err != nil {
				logger.Error("Failed to parse image URL", "url", im_url, "error", err)
				return
			}

			im_q := im_u.Query()

			if !im_q.Has("sha") {
				logger.Error("Image URL missing ?sha", "url", im_url)
				return
			}

			depiction_id = im_q.Get("sha")

			logger.Debug("Fetch image", "url", im_url)

			http_cl := harvest_http.NewClient()

			cache_opts := &http.GetWithCacheOptions{
				CheckLastModTime: cache_check_lastmod,
				Client:           http_cl,
				UserAgent:        "Mozilla/5.0 (Macintosh; Intel Mac OS X x.y; rv:10.0) Gecko/20100101 Firefox/10.0",
				BlobCache:        blob_c,
			}

			im_rsp, err := http.GetWithCacheAndOptions(ctx, cache_opts, im_url)

			if err != nil {
				logger.Error("Failed to retrieve image", "url", im_url, "error", err)
				return
			}

			im_body, err := io.ReadAll(im_rsp)
			im_rsp.Close()

			attrs := map[string]string{
				"type":               "image",
				"preview":            im_url,
				"subject_url":        row["URL"],
				"subject_title":      row["Title"],
				"subject_creditline": row["CreditLine"],
				"provider_name":      "Museum of Modern Art",
				"provider_url":       "https://www.moma.org/",
			}

			derive_opts := &harvest.DeriveEmbeddingsRecordsOptions{
				Provider:    "moma",
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
				logger.Error("Failed to write records buffer", "url", im_url, "error", err)
				return
			}

			logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
		})

		wr.Flush()
	}

	wg.Wait()

	err = wr.Close()

	if err != nil {
		log.Fatalf("Failed to close after writing, %v", err)
	}

}
