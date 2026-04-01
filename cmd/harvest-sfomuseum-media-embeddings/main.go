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

	parquet_go "github.com/parquet-go/parquet-go"
	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddingsdb"
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

	fs.StringVar(&iterator_uri, "iterator-uri", "repo://", "...")
	fs.StringVar(&iterator_source, "iterator-source", "/usr/local/data/sfomuseum-data-media-collection", "...")
	fs.StringVar(&provider, "provider", "sfomuseum-data-media-collection", "...")

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

	emb_cl, err := sfom_embeddings.NewEmbedder32(ctx, embeddings_client_uri)

	if err != nil {
		log.Fatalf("Failed to create embeddings client, %v", err)
	}

	var wr io.WriteCloser

	switch output {
	case "-":
		wr = os.Stdout
	default:

		w, err := os.OpenFile(output, os.O_RDWR|os.O_CREATE, 0644)

		if err != nil {
			log.Fatalf("Failed to open %s for writing, %v", output, err)
		}

		wr = w
	}

	p_wr := parquet_go.NewGenericWriter[*embeddingsdb.Record](wr)

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

		records := make([]*embeddingsdb.Record, 0)

		for _, m := range models {

			emb_req := &sfom_embeddings.EmbeddingsRequest{
				Model: m,
				Body:  im_body,
			}

			emb_rsp, err := emb_cl.ImageEmbeddings(ctx, emb_req)

			if err != nil {
				logger.Error("Failed to derive ebeddings", "image", im_url, "model", m, "error", err)
				continue
			}

			db_rec := &embeddingsdb.Record{
				Provider:    provider,
				DepictionId: depiction_id,
				SubjectId:   subject_id,
				Model:       emb_rsp.Model(),
				Embeddings:  emb_rsp.Embeddings(),
				Attributes: map[string]string{
					"uri": im_url,
					// "title": title,
				},
				Created: emb_rsp.Created(),
			}

			logger.Debug("Add record", "key", db_rec.Key())
			records = append(records, db_rec)
		}

		if len(records) > 0 {

			_, err = p_wr.Write(records)

			if err != nil {
				logger.Error("Failed to write records", "url", im_url, "error", err)
			}

			logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
		}
	}

	//

	p_wr.Flush()

	err = p_wr.Close()

	if err != nil {
		log.Fatalf("Failed to close Parquet writer, %v", err)
	}

	switch output {
	case "-":
		// pass
	default:
		err = wr.Close()

		if err != nil {
			log.Fatalf("Failed to close %s after writing, %v", output, err)
		}
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
