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

type ObjectInfo struct {
	Title      string
	Creditline string
}

func main() {

	var embeddings_client_uri string
	var cache_uri string
	var cache_check_lastmod bool

	var objects string
	var published_images string

	var workers int
	var output string
	var verbose bool
	var models multi.MultiCSVString

	fs := flagset.NewFlagSet("nga")

	fs.StringVar(&objects, "objects", "", "The path to the 'objects.csv' file contained in the NationalGalleryOfArt/opendata GitHub repository.")
	fs.StringVar(&published_images, "published-images", "", "The path to the 'published_images.csv' file contained in the NationalGalleryOfArt/opendata GitHub repository.")

	fs.IntVar(&workers, "workers", 5, "The number of workers to use to fetch images (and derive embeddings) concurrently")
	fs.Var(&models, "model", "One or more models to derive embeddings for. This may also be a comma-separated list.")

	fs.StringVar(&output, "output", "-", "The path where Parquet-encoded data should be written. If \"-\" then data will be written to STDOUT.")
	fs.StringVar(&embeddings_client_uri, "embeddings-client-uri", "mobileclip://?client-uri=grpc://localhost:8080", "A registered sfomuseum/go-embeddingsdb/client.Client URI.")

	fs.StringVar(&cache_uri, "cache-uri", "null://", "A register gocloud.dev/blob.Bucket URI to use for caching images. If null:// then no images will be cached.")
	fs.BoolVar(&cache_check_lastmod, "cache-check-lastmod", true, "A boolean value to indicate whether the last modified date of an object to harvest should be compared against the local cache.")

	fs.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging.")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Generate Parquet-encoded embeddings from the National Gallery of Art (NGA) open data release.\n")
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

	objects_r, err := csvdict.NewReaderFromPath(objects)

	if err != nil {
		log.Fatalf("Failed to create CSV reader for objects, %v", err)
	}

	images_r, err := csvdict.NewReaderFromPath(published_images)

	if err != nil {
		log.Fatalf("Failed to create CSV reader for images, %v", err)
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

	// First, iterate through all the objects and capture title and creditline information
	// for inclusion below

	objects_wg := new(sync.WaitGroup)
	objects_lookup := new(sync.Map)

	for row, err := range objects_r.Iterate() {

		if err != nil {
			log.Fatalf("Objects iterator yielded an error, %v", err)
		}

		objects_wg.Go(func() {

			objects_lookup.Store(row["objectid"], &ObjectInfo{
				Title:      row["title"],
				Creditline: row["creditline"],
			})
		})
	}

	objects_wg.Wait()

	// Now fetch images

	throttle := make(chan bool, workers)

	for i := 0; i < workers; i++ {
		throttle <- true
	}

	wg := new(sync.WaitGroup)

	for row, err := range images_r.Iterate() {

		if err != nil {
			log.Fatalf("Images iterator yielded an error, %v", err)
		}

		<-throttle

		wg.Go(func() {

			defer func() {
				throttle <- true
			}()

			count += 1

			logger := slog.Default()
			logger = logger.With("path", row["uuid"])

			depiction_id := row["uuid"]
			subject_id := row["depictstmsobjectid"]
			im_url := row["iiifthumburl"]

			logger.Debug("Fetch image", "url", im_url)

			http_cl := harvest_http.NewClient()

			cache_opts := &http.GetWithCacheOptions{
				CheckLastModTime: cache_check_lastmod,
				Client:           http_cl,
				UserAgent:        "Mozilla/5.0 (Macintosh; Intel Mac OS X x.y; rv:10.0) Gecko/20100101 Firefox/10.0",
				BlobCache: blob_c,
			}

			im_r, err := http.GetWithCacheAndOptions(ctx, cache_opts, im_url)

			if err != nil {
				logger.Error("Failed to retrieve image", "url", im_url, "error", err)
				return
			}

			if im_r == nil {
				return
			}

			im_body, err := io.ReadAll(im_r)
			im_r.Close()

			if err != nil {
				logger.Error("Failed to read image", "url", im_url, "error", err)
				return
			}

			// works: https://www.nga.gov/artworks/12198-symphony-white-no-1-white-girl
			// does not work: https://www.nga.gov/artworks/12198
			// works: purl.org/nga/collection/artobject/12198
			// see also: https://github.com/NationalGalleryOfArt/opendata/issues/19

			attrs := map[string]string{
				"type":               "image",
				"preview":            im_url,
				"subject_url":        fmt.Sprintf("https://purl.org/nga/collection/artobject/%s", subject_id),
				"subject_title":      "",
				"subject_creditline": "",
				"provider_name":      "National Gallery of Art",
				"provider_url":       "https://www.nga.gov/",
			}

			v, exists := objects_lookup.Load(subject_id)

			if !exists {
				logger.Warn("Unable to load object info", "object id", subject_id)
			} else {
				obj_info := v.(*ObjectInfo)
				attrs["subject_title"] = obj_info.Title
				attrs["subject_creditline"] = obj_info.Creditline
			}

			derive_opts := &harvest.DeriveEmbeddingsRecordsOptions{
				Provider:    "nga",
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
