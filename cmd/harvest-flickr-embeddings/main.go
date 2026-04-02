package main

/*

./bin/harvest-flickr-embeddings -flickr-client-uri file:///usr/local/sfomuseum/lockedbox/flickr/aaronofsfo.txt -param user_id=49487266@N07 -param method=flickr.photosets.getPhotos -param photoset_id=flickr.photosets.getPhotos -provider flickr-49487266@N07 -spr-path photoset.photo -verbose -model s0 -model s1 -model s2 -output test4.parquet


*/

import (
	"context"
	_ "fmt"
	"io"
	"log"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"github.com/aaronland/go-flickr-api/client"
	"github.com/aaronland/gocloud/runtimevar"
	parquet_go "github.com/parquet-go/parquet-go"
	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest/flickr"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
	"github.com/tidwall/gjson"
)

func main() {

	var flickr_client_uri string
	var embeddings_client_uri string

	var provider string
	var spr_path string

	var params multi.KeyValueString

	var output string
	var verbose bool
	var models multi.MultiCSVString

	fs := flagset.NewFlagSet("flickr")
	fs.StringVar(&flickr_client_uri, "flickr-client-uri", "", "A gocloud/runtimevar URI which dereferences in to a valid aaronland/go-flickr-api/client.Client URI.")
	fs.Var(&params, "param", "Zero or more {KEY}={VALUE} parameters to query the Flickr API with.")

	fs.StringVar(&provider, "provider", "flickr", "The name of the provider to assign to each embeddings record.")
	fs.StringVar(&spr_path, "spr-path", "", "The path to the list of photos in the Flickr API response. Paths should be described using tidwall/gjson \"dot\" notation.")

	fs.Var(&models, "model", "One or more models to use to generate embeddings. This may also be a comma-separated string containing a list of models.")

	fs.StringVar(&output, "output", "-", "The path where Parquet-encoded data should be written. If \"-\" then data will be written to STDOUT.")
	fs.StringVar(&embeddings_client_uri, "embeddings-client-uri", "mobileclip://?client-uri=grpc://localhost:8080", "A registered sfomuseum/go-embeddings.Client URI.")
	fs.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging.")

	flagset.Parse(fs)

	if verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Verbose logging enabled")
	}

	logger := slog.Default()
	ctx := context.Background()

	if len(models) == 0 {
		log.Fatal("No models defined")
	}

	client_uri, err := runtimevar.StringVar(ctx, flickr_client_uri)

	if err != nil {
		log.Fatalf("Failed to derive Flickr client URI, %v", err)
	}

	flickr_cl, err := client.NewClient(ctx, client_uri)

	if err != nil {
		log.Fatalf("Failed to create new Flickr client, %v", err)
	}

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

	emb_opts := &flickr.EmbeddingsForFlickrSPROptions{
		EmbeddingsClient: emb_cl,
		ParquetWriter:    p_wr,
		Models:           models,
		Provider:         provider,
	}

	emb_cb := flickr.EmbeddingsForFlickrSPRPaginatedCallbackFunc(emb_opts, spr_path)

	args := &url.Values{}

	for _, kv := range params {
		args.Set(kv.Key(), kv.Value().(string))
	}

	if args.Has("userid") {

		userid := args.Get("userid")

		if strings.HasPrefix(userid, "nsid:") {

			username := strings.Replace(userid, "nsid:", "", 1)
			logger.Debug("Derive NSID for user", "username", username)

			args := &url.Values{}
			args.Set("method", "flickr.people.findByUsername")
			args.Set("username", username)

			rsp, err := flickr_cl.ExecuteMethod(ctx, args)

			if err != nil {
				log.Fatalf("Failed to execute method, %v", err)
			}

			defer rsp.Close()

			body, err := io.ReadAll(rsp)

			if err != nil {
				log.Fatalf("Failed to read response, %v", err)
			}

			nsid_rsp := gjson.GetBytes(body, "user.nsid")

			if !nsid_rsp.Exists() {
				log.Fatalf("Failed to derive NSID")
			}

			nsid := nsid_rsp.String()
			logger.Debug("Set userid parameter", "username", username, "nsid", nsid)

			args.Del("userid")
			args.Set("userid", nsid)
		}
	}

	err = client.ExecuteMethodPaginatedWithClient(ctx, flickr_cl, args, emb_cb)

	if err != nil {
		log.Fatalf("Failed to execute paginated method, %v", err)
	}

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
