package main

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
	"github.com/tidwall/gjson"
	"github.com/whosonfirst/go-whosonfirst-feature/properties"
	"github.com/whosonfirst/go-whosonfirst-iterate/v3"
)

func main() {

	var embeddings_client_uri string
	var iterator_uri string
	var iterator_source string

	var output string
	var verbose bool
	var models multi.MultiCSVString

	fs := flagset.NewFlagSet("flickr")

	fs.StringVar(&iterator_uri, "iterator-uri", "repo://?exclude=properties.edtf:deprecated=.*", "A registered go-whosonfirst-iterate/v3.Iterator URI.")
	fs.StringVar(&iterator_source, "iterator-source", "/usr/local/data/sfomuseum-data-socialmedia-instagram", "The source for the go-whosonfirst-iterate/v3.Iterator instance to process.")

	fs.Var(&models, "model", "One or more models to derive embeddings for. This may also be a comma-separated list.")

	fs.StringVar(&output, "output", "-", "The path where Parquet-encoded data should be written. If \"-\" then data will be written to STDOUT.")
	fs.StringVar(&embeddings_client_uri, "embeddings-client-uri", "mobileclip://?client-uri=grpc://localhost:8080", "A registered sfomuseum/go-embeddingsdb/client.Client URI.")
	fs.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging.")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Generate Parquet-encoded embeddings from SFO Museum sfomuseum-data-socialmedia-instagram data repositories (aka \"Instragram photos\").\n")
		fmt.Fprintf(os.Stderr, "Usage:\n\t%s [options]", os.Args[0])
		fmt.Fprintf(os.Stderr, "Valid options are:\n")
		fs.PrintDefaults()
	}

	flagset.Parse(fs)

	if verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Verbose logging enabled")
	}

	logger := slog.Default()
	logger = logger.With("output", output)

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

	iter, err := iterate.NewIterator(ctx, iterator_uri)

	if err != nil {
		log.Fatalf("Failed to create iterator, %v", err)
	}

	provider := "sfomuseum-socialmedia-instagram"

	for rec, err := range iter.Iterate(ctx, iterator_source) {

		if err != nil {
			log.Fatalf("Iterator yielded an error, %v", err)
		}

		logger := slog.Default()
		logger = logger.With("path", rec.Path)

		body, err := io.ReadAll(rec.Body)
		rec.Body.Close()

		if err != nil {
			logger.Error("Failed to read record body", "error", err)
			continue
		}

		id, err := properties.Id(body)

		if err != nil {
			logger.Error("Failed to derive ID", "error", err)
			continue
		}

		logger = logger.With("id", id)

		name, err := properties.Name(body)

		if err != nil {
			logger.Error("Failed to derive name", "error", err)
			continue
		}

		id_rsp := gjson.GetBytes(body, "properties.instagram:post.media_id")

		if !id_rsp.Exists() {
			logger.Error("Failed to derive media ID")
			continue
		}

		media_id := id_rsp.String()
		logger = logger.With("media id", media_id)

		taken_rsp := gjson.GetBytes(body, "properties.instagram:post.taken_at")

		if !taken_rsp.Exists() {
			logger.Error("Failed to derive taken at date")
			continue
		}

		taken_at := taken_rsp.String()

		depiction_id := strconv.FormatInt(id, 10)
		subject_id := media_id

		// Update to use https://github.com/sfomuseum/go-sfomuseum-instagram-publish/blob/main/secret/secret.go
		ig_secret := instagramSecret(media_id)

		im_url := fmt.Sprintf("https://static.sfomuseum.org/media/instagram/%s/%s_%s_n.jpg", media_id, media_id, ig_secret)
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
			"type":               "image",
			"preview":            im_url,
			"subject_url":        fmt.Sprintf("https://millsfield.sfomuseum.org/instagram/%s", subject_id),
			"subject_title":      name,
			"subject_creditline": fmt.Sprintf("SFO Museum Instagram post from %s", taken_at),
			"provider_name":      "SFO Museum",
			"provider_url":       "https://millsfield.sfomuseum.org/instagram",
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
			logger.Warn("Failed to derive any embeddings records")
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

func instagramSecret(media_id string) string {

	b := []byte(media_id)

	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}

	reversed := string(b)
	hash := md5.Sum([]byte(reversed))

	hex := fmt.Sprintf("%x", hash)

	if len(hex) < 10 {
		return hex
	}

	return hex[:10]
}
