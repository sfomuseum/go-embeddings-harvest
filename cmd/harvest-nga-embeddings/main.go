package main

import (
	"context"
	"io"
	"fmt"
	"os"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/sfomuseum/go-csvdict/v2"
	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
)

func main() {

	var embeddings_client_uri string
	var published_images string

	var output string
	var verbose bool
	var models multi.MultiCSVString

	fs := flagset.NewFlagSet("nga")

	fs.StringVar(&published_images, "published-images", "", "The path to the 'published_images.csv' file contained in the NationalGalleryOfArt/opendata GitHub repository.")

	fs.Var(&models, "model", "One or more models to derive embeddings for. This may also be a comma-separated list.")

	fs.StringVar(&output, "output", "-", "The path where Parquet-encoded data should be written. If \"-\" then data will be written to STDOUT.")
	fs.StringVar(&embeddings_client_uri, "embeddings-client-uri", "mobileclip://?client-uri=grpc://localhost:8080", "A registered sfomuseum/go-embeddingsdb/client.Client URI.")
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

	wr, err := harvest.NewWriter(ctx, output)

	if err != nil {
		log.Fatalf("Failed to create new writer, %v", err)
	}

	csv_r, err := csvdict.NewReaderFromPath(published_images)

	if err != nil {
		log.Fatalf("Failed to create CSV reader, %v", err)
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

	for row, err := range csv_r.Iterate() {

		if err != nil {
			log.Fatalf("Iterator yielded an error, %v", err)
		}

		count += 1

		logger := slog.Default()
		logger = logger.With("path", row["uuid"])

		depiction_id := row["uuid"]
		subject_id := row["depictstmsobjectid"]
		im_url := row["iiifthumburl"]

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
			continue
		}

		if len(records) == 0 {
			logger.Warn("Failed to derive embeddings")
			continue
		}

		_, err = wr.Write(records)

		if err != nil {
			logger.Error("Failed to write records", "url", im_url, "error", err)
		}

		logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
	}

	//

	err = wr.Close()

	if err != nil {
		log.Fatalf("Failed to close after writing, %v", err)
	}

}
