package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"

	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddingsdb/parquet"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
	"github.com/tidwall/gjson"
	"github.com/sfomuseum/go-embeddings-harvest"		
	"github.com/whosonfirst/go-whosonfirst-feature/properties"
	"github.com/whosonfirst/go-whosonfirst-iterate/v3"
	"github.com/whosonfirst/go-whosonfirst-uri"
)

func main() {

	var embeddings_client_uri string
	var iterator_uri string
	var iterator_source string

	var workers int
	var output string
	var verbose bool
	var models multi.MultiCSVString

	fs := flagset.NewFlagSet("flickr")

	fs.StringVar(&iterator_uri, "iterator-uri", "repo://?exclude=properties.edtf:deprecated=.*", "A registered go-whosonfirst-iterate/v3.Iterator URI.")
	fs.StringVar(&iterator_source, "iterator-source", "/usr/local/data/sfomuseum-data-media", "The source for the go-whosonfirst-iterate/v3.Iterator instance to process.")

	fs.IntVar(&workers, "workers", 5, "The number of workers to use to fetch images (and derive embeddings) concurrently")
	fs.Var(&models, "model", "One or more models to derive embeddings for. This may also be a comma-separated list.")

	fs.StringVar(&output, "output", "-", "The path where Parquet-encoded data should be written. If \"-\" then data will be written to STDOUT.")
	fs.StringVar(&embeddings_client_uri, "embeddings-client-uri", "mobileclip://?client-uri=grpc://localhost:8080", "A registered sfomuseum/go-embeddingsdb/client.Client URI.")
	fs.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging.")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Generate Parquet-encoded embeddings from SFO Museum sfomuseum-data-media* data repositorie.\n")
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

	wr, err := parquet.NewWriter(ctx, output)

	if err != nil {
		log.Fatalf("Failed to create writers, %v", err)
	}

	//

	iter, err := iterate.NewIterator(ctx, iterator_uri)

	if err != nil {
		log.Fatalf("Failed to create iterator, %v", err)
	}

	throttle := make(chan bool, workers)

	for i := 0; i < workers; i++ {
		throttle <- true
	}

	wg := new(sync.WaitGroup)

	for rec, err := range iter.Iterate(ctx, iterator_source) {

		if err != nil {
			log.Fatalf("Iterator yielded an error, %v", err)
		}

		<-throttle

		wg.Go(func() {

			defer func() {
				throttle <- true
			}()

			logger := slog.Default()
			logger = logger.With("path", rec.Path)

			id, uri_args, err := uri.ParseURI(rec.Path)

			if err != nil {
				logger.Error("Failed to parse path", "error", err)
				return
			}

			if uri_args.IsAlternate {
				return
			}

			logger = logger.With("id", id)

			body, err := io.ReadAll(rec.Body)
			rec.Body.Close()

			if err != nil {
				logger.Error("Failed to read record body", "error", err)
				return
			}

			parent_id, err := properties.ParentId(body)

			if err != nil {
				logger.Error("Failed to derive parent ID", "error", err)
				return
			}

			name, err := properties.Name(body)

			if err != nil {
				logger.Error("Failed to derive name", "error", err)
				return
			}

			depiction_id := strconv.FormatInt(id, 10)
			subject_id := strconv.FormatInt(parent_id, 10)

			secret_rsp := gjson.GetBytes(body, "properties.media:properties.sizes.n.secret")

			if !secret_rsp.Exists() {
				logger.Error("Failed to derive image secret")
				return
			}

			secret := secret_rsp.String()

			tree, err := uri.Id2Path(id)

			if err != nil {
				logger.Error("Failed to derive image tree", "error", err)
				return
			}

			im_url := fmt.Sprintf("https://static.sfomuseum.org/media/%s/%d_%s_n.jpg", tree, id, secret)

			logger.Debug("Fetch image", "url", im_url)

			im_rsp, err := http.Get(im_url)

			if err != nil {
				logger.Error("Failed to retrieve image", "url", im_url, "error", err)
				return
			}

			im_body, err := io.ReadAll(im_rsp.Body)
			im_rsp.Body.Close()

			if err != nil {
				logger.Error("Failed to read image", "url", im_url, "error", err)
				return
			}

			subject_url := fmt.Sprintf("https://millsfield.sfomuseum.org/id/%s", subject_id)
			subject_creditline := "SFO Museum"

			attrs := map[string]string{
				"type":               "image",
				"preview":            im_url,
				"subject_url":        subject_url,
				"subject_title":      name,
				"subject_creditline": subject_creditline,
				"provider_name":      "SFO Museum",
				"provider_url":       "https://collection.sfomuseum.org",
			}

			derive_opts := &harvest.DeriveEmbeddingsRecordsOptions{
				Provider:    "sfomuseum-data-media",
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
				logger.Warn("No embeddings records produced")
				return
			}

			_, err = wr.Write(records)

			if err != nil {
				logger.Error("Failed to write records", "url", im_url, "error", err)
			}

			logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
		})

		wr.Flush()
	}

	wg.Wait()

	err = wr.Close()

	if err != nil {
		log.Fatalf("Failed to close writers, %v", err)
	}

}
