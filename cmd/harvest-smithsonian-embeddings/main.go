package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	_ "regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/aaronland/go-json-query"
	jw "github.com/aaronland/go-jsonl/walk"
	"github.com/aaronland/go-smithsonian-openaccess"
	"github.com/aaronland/go-smithsonian-openaccess/walk"
	"github.com/sfomuseum/go-blobcache"
	"github.com/sfomuseum/go-blobcache/http"
	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest"
	"github.com/sfomuseum/go-embeddingsdb/parquet"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
	"github.com/tidwall/gjson"
	"gocloud.dev/blob"
)

func main() {

	var embeddings_client_uri string
	var cache_uri string
	var cache_check_lastmod bool

	var bucket_uri string

	var workers int
	var output string
	var verbose bool
	var models multi.MultiCSVString
	var units multi.MultiCSVString

	fs := flagset.NewFlagSet("nga")

	fs.StringVar(&bucket_uri, "bucket-uri", "si://", "A valid GoCloud bucket URI. Valid schemes are: file://, s3:// and si:// which is signals that data should be retrieved from the Smithsonian's 'smithsonian-open-access' S3 bucket.")

	fs.IntVar(&workers, "workers", 5, "The number of workers to use to fetch images (and derive embeddings) concurrently")
	fs.Var(&models, "model", "One or more models to derive embeddings for. This may also be a comma-separated list.")
	fs.Var(&units, "unit", "...")

	fs.StringVar(&cache_uri, "cache-uri", "null://", "A register gocloud.dev/blob.Bucket URI to use for caching images. If null:// then no images will be cached.")
	fs.BoolVar(&cache_check_lastmod, "cache-check-lastmod", false, "A boolean value to indicate whether the last modified date of an object to harvest should be compared against the local cache.")

	fs.StringVar(&output, "output", "", "The path where Parquet-encoded data should be written. If \"-\" then data will be written to STDOUT.")
	fs.StringVar(&embeddings_client_uri, "embeddings-client-uri", "mobileclip://?client-uri=grpc://localhost:8080", "A registered sfomuseum/go-embeddingsdb/client.Client URI.")
	fs.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging.")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Generate Parquet-encoded embeddings from the Smithsonian (SI) open data release.\n")
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

	ctx, bucket, err := openaccess.OpenBucket(ctx, bucket_uri)

	if err != nil {
		log.Fatalf("Failed to open bucket, %v", err)
	}

	defer bucket.Close()

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
				slog.Info("Processed rows", "count", atomic.LoadInt64(&count))
			}
		}
	}()

	throttle := make(chan bool, workers)

	for i := 0; i < workers; i++ {
		throttle <- true
	}

	wg := new(sync.WaitGroup)

	walk_cb := func(ctx context.Context, rec *jw.WalkRecord, err error) error {

		if err != nil {
			slog.Error("Walk callback reported an error", "error", err)
			return err
		}

		<-throttle

		wg.Go(func() {

			defer func() {
				atomic.AddInt64(&count, 1)
				throttle <- true
			}()

			logger := slog.Default()
			logger = logger.With("lineno", rec.LineNumber)

			media_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.online_media.media")
			media := media_rsp.Array()
			count_m := len(media)

			if count_m == 0 {
				return
			}

			logger.Debug("Process record", "count", count_m)

			title_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.title.content")
			subject_title := title_rsp.String()

			if subject_title == "" {
				logger.Warn("Record is missing title")
				return
			}

			subject_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.record_ID")
			subject_id := subject_rsp.String()

			if subject_id == "" {
				logger.Warn("Record is missing ID")
				return
			}

			logger = logger.With("subject", subject_id)

			link_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.record_link")
			subject_url := link_rsp.String()

			if subject_url == "" {
				// NMAH doesn't always have record_link?
				guid_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.guid")
				subject_url = guid_rsp.String()
			}
			
			if subject_url == "" {
				logger.Warn("Record is missing link")
				// return
			}

			unit_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.unit_code")
			unit := unit_rsp.String()

			if unit == "" {
				logger.Warn("Record is missing unit")
				return
			}

			unit = strings.ToLower(unit)
			logger = logger.With("unit", unit)

			source_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.data_source")
			provider_name := source_rsp.String()

			if provider_name == "" {
				logger.Warn("Record is missing provider")
				return
			}

			credit_rsp := gjson.GetBytes(rec.Body, "content.freetext.creditLine.0.content")
			subject_creditline := credit_rsp.String()

			if subject_creditline == "" {
				logger.Warn("Record is missing creditline.")
				// return
			}

			for _, m := range media {

				depiction_rsp := m.Get("id")
				depiction_id := depiction_rsp.String()

				if depiction_id == "" {
					logger.Warn("Media item is missing depiction ID")
					continue
				}

				im_rsp := m.Get("content")
				im_url := im_rsp.String()

				if im_url == "" {
					logger.Warn("Media item is missing image URL (content)")
					continue
				}

				im_url = fmt.Sprintf("%s_screen", im_url)

				im_body, err := http.GetBytesWithCacheAndOptions(ctx, cache_opts, im_url)

				if err != nil {
					logger.Error("Failed to retrieve image", "url", im_url, "error", err)
					continue
				}

				attrs := map[string]string{
					"type":               "image",
					"preview":            im_url,
					"subject_url":        subject_url,
					"subject_title":      subject_title,
					"subject_creditline": subject_creditline,
					"provider_name":      provider_name,
					"provider_url":       fmt.Sprintf("https://si.edu#%s", unit),
				}

				derive_opts := &harvest.DeriveEmbeddingsRecordsOptions{
					Provider:    unit,
					DepictionId: depiction_id,
					SubjectId:   subject_id,
					Attributes:  attrs,
					Models:      models,
					Body:        im_body,
				}

				records, err := harvest.DeriveEmbeddingsRecords(ctx, emb_cl, derive_opts)

				if err != nil {
					logger.Error("Failed to derive embeddings records", "error", err)
					continue
				}

				if len(records) == 0 {
					logger.Warn("Failed to derive embeddings")
					continue
				}

				_, err = wr.Write(records)

				if err != nil {
					logger.Error("Failed to write records buffer", "url", im_url, "error", err)
					continue
				}

				logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
			}
		})

		return nil
	}

	// TBD: this is not working as expected; filter on media item count in callback for now

	/*
		qs := &query.QuerySet{
			Queries: []*query.Query{
				&query.Query{
					Path:  "content.descriptiveNonRepeating.online_media",
					Match: regexp.MustCompile(`/^\.*$/`),
				},
			},
			Mode: query.QUERYSET_MODE_ALL,
		}
	*/

	for _, unit := range units {

		uri := fmt.Sprintf("metadata/edan/%s", unit)
		slog.Debug("Query unit", "uri", uri)

		b := blob.PrefixedBucket(bucket, uri)

		opts := &walk.WalkOptions{
			Callback: walk_cb,
			IsBzip:   false,
			// QuerySet: qs,
		}

		err := walk.WalkBucket(ctx, opts, b)

		if err != nil {
			log.Fatalf("Failed to crawl %s, %v", uri, err)
		}

		wr.Flush()
	}

	wg.Wait()

	err = wr.Close()

	if err != nil {
		log.Fatalf("Failed to close after writing, %v", err)
	}

}
