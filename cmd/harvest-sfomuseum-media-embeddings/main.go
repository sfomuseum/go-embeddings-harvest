package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"strconv"

	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
	"github.com/tidwall/gjson"
	"github.com/whosonfirst/go-whosonfirst-feature/properties"
	"github.com/whosonfirst/go-whosonfirst-iterate/v3"
	"github.com/whosonfirst/go-whosonfirst-uri"
)

func main() {

	var embeddings_client_uri string
	var iterator_uri string
	var iterator_source string
	var provider string

	var output string
	var verbose bool
	var models multi.MultiCSVString

	fs := flagset.NewFlagSet("flickr")

	fs.StringVar(&iterator_uri, "iterator-uri", "repo://?exclude=properties.edtf:deprecated=.*", "A registered go-whosonfirst-iterate/v3.Iterator URI.")
	fs.StringVar(&iterator_source, "iterator-source", "/usr/local/data/sfomuseum-data-media-collection", "The source for the go-whosonfirst-iterate/v3.Iterator instance to process.")
	fs.StringVar(&provider, "provider", "sfomuseum-data-media-collection", "The name of the provider to assign to each embeddings record.")

	fs.Var(&models, "model", "One or more models to derive embeddings for. This may also be a comma-separated list.")

	fs.StringVar(&output, "output", "-", "The path where Parquet-encoded data should be written. If \"-\" then data will be written to STDOUT.")
	fs.StringVar(&embeddings_client_uri, "embeddings-client-uri", "mobileclip://?client-uri=grpc://localhost:8080", "A registered sfomuseum/go-embeddingsdb/client.Client URI.")
	fs.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging.")

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

	wr, err := harvest.NewWriter(output)

	if err != nil {
		log.Fatalf("Failed to create writers, %v", err)
	}

	//

	iter, err := iterate.NewIterator(ctx, iterator_uri)

	if err != nil {
		log.Fatalf("Failed to create iterator, %v", err)
	}

	for rec, err := range iter.Iterate(ctx, iterator_source) {

		if err != nil {
			log.Fatalf("Iterator yielded an error, %v", err)
		}

		logger := slog.Default()
		logger = logger.With("path", rec.Path)

		id, uri_args, err := uri.ParseURI(rec.Path)

		if err != nil {
			logger.Error("Failed to parse path", "error", err)
			continue
		}

		if uri_args.IsAlternate {
			continue
		}

		body, err := io.ReadAll(rec.Body)
		rec.Body.Close()

		if err != nil {
			logger.Error("Failed to read record body", "error", err)
			continue
		}

		parent_id, err := properties.ParentId(body)

		if err != nil {
			logger.Error("Failed to derive parent ID", "error", err)
			continue
		}

		logger = logger.With("id", id)

		depiction_id := strconv.FormatInt(id, 10)
		subject_id := strconv.FormatInt(parent_id, 10)

		secret_rsp := gjson.GetBytes(body, "properties.media:properties.sizes.n.secret")

		if !secret_rsp.Exists() {
			logger.Error("Failed to derive image secret")
			continue
		}

		secret := secret_rsp.String()

		tree, err := uri.Id2Path(id)

		if err != nil {
			logger.Error("Failed to derive image tree", "error", err)
			continue
		}

		im_url := fmt.Sprintf("https://static.sfomuseum.org/media/%s/%d_%s_n.jpg", tree, id, secret)

		logger.Debug("Fetch image", "url", im_url)

		im_rsp, err := http.Get(im_url)

		if err != nil {
			logger.Error("Failed to retrieve image", "url", im_url, "error", err)
			continue
		}

		im_body, err := io.ReadAll(im_rsp.Body)
		im_rsp.Body.Close()

		if err != nil {
			logger.Error("Failed to read image", "url", im_url, "error", err)
			continue
		}

		attrs := map[string]string{
			"uri": im_url,
		}

		derive_opts := &harvest.DeriveEmbeddingsRecordsOptions{
			Provider:    provider,
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
			logger.Warn("No embeddings records produced")
			continue
		}

		_, err = wr.ParquetWriter.Write(records)

		if err != nil {
			logger.Error("Failed to write records", "url", im_url, "error", err)
		}

		logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
	}

	err = wr.Close()

	if err != nil {
		log.Fatalf("Failed to close writers, %v", err)
	}

}
