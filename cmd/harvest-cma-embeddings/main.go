package main

// go run cmd/harvest-cma-embeddings/main.go -objects /usr/local/data/cma/openaccess/data.csv -model s1 -output work/cma.parquet -verbose

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sfomuseum/go-blobcache"
	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-csvdict/v2"
	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest"
	"github.com/sfomuseum/go-embeddingsdb/parquet"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
	"github.com/tidwall/gjson"
)

func main() {

	var embeddings_client_uri string
	var cache_uri string
	var cache_check_lastmod bool

	var objects string

	var workers int
	var output string
	var verbose bool
	var models multi.MultiCSVString

	fs := flagset.NewFlagSet("cma")

	fs.StringVar(&objects, "objects", "", "The path to the 'data.csv' file contained in the ClevelandMuseumArt/openaccess GitHub repository.")
	fs.IntVar(&workers, "workers", 5, "The number of workers to use to fetch images (and derive embeddings) concurrently")
	fs.Var(&models, "model", "One or more models to derive embeddings for. This may also be a comma-separated list.")

	fs.StringVar(&output, "output", "", "The path where Parquet-encoded data should be written. If \"-\" then data will be written to STDOUT.")
	fs.StringVar(&embeddings_client_uri, "embeddings-client-uri", "mobileclip://?client-uri=grpc://localhost:8080", "A registered sfomuseum/go-embeddingsdb/client.Client URI.")

	fs.StringVar(&cache_uri, "cache-uri", "null://", "A register gocloud.dev/blob.Bucket URI to use for caching images. If null:// then no images will be cached.")
	fs.BoolVar(&cache_check_lastmod, "cache-check-lastmod", false, "A boolean value to indicate whether the last modified date of an object to harvest should be compared against the local cache.")

	fs.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging.")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Generate Parquet-encoded embeddings from the Cleveland Museum of Art (CMA) open data release.\n")
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

	http_cl := http.NewClient()

	cache_opts := &http.GetWithCacheOptions{
		CheckLastModTime: cache_check_lastmod,
		Client:           http_cl,
		BlobCache:        blob_c,
	}

	wr, err := parquet.NewWriter(ctx, output)

	if err != nil {
		log.Fatalf("Failed to create new writer, %v", err)
	}

	objects_r, err := csvdict.NewReaderFromPath(objects)

	if err != nil {
		log.Fatalf("Failed to create CSV reader for objects, %v", err)
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

	/*


		for records, err := harvester.Iterate() {

			if err != nil {
				log.Fatal(err)
			}

			), err = wr.Write(records)
		}

	*/

	wg := new(sync.WaitGroup)

	for row, err := range objects_r.Iterate() {

		if err != nil {
			log.Fatalf("Objects iterator yielded an error, %v", err)
		}

		<-throttle

		wg.Go(func() {

			defer func() {
				throttle <- true
			}()

			count += 1

			logger := slog.Default()

			if row["image_web"] == "" {
				return
			}

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

			for _, im_url := range images {

				fname := filepath.Base(im_url)

				depiction_id := fmt.Sprintf("%s#%s", row["accession_number"], fname)
				subject_id := row["accession_number"]

				logger = logger.With("subject", subject_id)
				logger = logger.With("depiction", depiction_id)

				logger.Debug("Fetch image", "url", im_url)

				im_body, err := http.GetBytesWithCacheAndOptions(ctx, cache_opts, im_url)

				if err != nil {
					logger.Error("Failed to retrieve image", "url", im_url, "error", err)
					return
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

				derive_opts := &harvest.DeriveEmbeddingsRecordsOptions{
					Provider:    "cma",
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
			}
		})

		wr.Flush()
	}

	wg.Wait()

	err = wr.Close()

	if err != nil {
		log.Fatalf("Failed to close after writing, %v", err)
	}

}
