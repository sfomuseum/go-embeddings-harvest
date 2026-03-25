package flickr

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/aaronland/go-flickr-api/client"
	parquet_go "github.com/parquet-go/parquet-go"
	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/tidwall/gjson"
)

// EmbeddingsForFlickrSPROptions defined configuration options to be passed to the various "EmbeddingsForFlickrSPR*" methods.
type EmbeddingsForFlickrSPROptions struct {
	// The name of the provider to assign to each embedding.
	Provider string
	// The list of models to derive embeddings from.
	Models []string
	// The [sfom_embeddings.Embedder[float32]] instance to derive embeddings with.
	EmbeddingsClient sfom_embeddings.Embedder[float32]
	// The [*parquet_go.GenericWriter[*embeddingsdb.Record]] instances used to write embeddings data to.
	ParquetWriter *parquet_go.GenericWriter[*embeddingsdb.Record]
}

// EmbeddingsForFlickrSPRPaginatedCallbackFunc returns a [go-flickr-api/client.ExecuteMethodPaginatedCallback] function
// to derive embeddings for all the photos derived from 'spr_path'.
func EmbeddingsForFlickrSPRPaginatedCallbackFunc(emb_opts *EmbeddingsForFlickrSPROptions, spr_path string) client.ExecuteMethodPaginatedCallback {

	fn := func(ctx context.Context, r io.ReadSeekCloser, err error) error {

		if err != nil {
			return err
		}

		defer r.Close()

		return EmbeddingsForFlickrSPRReader(ctx, emb_opts, r, spr_path)
	}

	return fn
}

// EmbeddingsForFlickrSPRReader derives embeddings for a list of Flickr SPR results identified by 'spr_path' and encoded in 'r'.
func EmbeddingsForFlickrSPRReader(ctx context.Context, emb_opts *EmbeddingsForFlickrSPROptions, r io.Reader, spr_path string) error {

	body, err := io.ReadAll(r)

	if err != nil {
		return fmt.Errorf("Failed to read response body, %w", err)
	}

	photos_rsp := gjson.GetBytes(body, spr_path)

	if !photos_rsp.Exists() {
		// slog.Debug("Response body missing SPR path", "path", spr_path, "body", string(body))
		return fmt.Errorf("Response body missing '%s' path", spr_path)
	}

	err = EmbeddingsForFlickrSPRArray(ctx, emb_opts, photos_rsp)

	if err != nil {
		return fmt.Errorf("Failed to derive embeddings for SPR array, %w", err)
	}

	return nil
}

// EmbeddingsForFlickrSPRArray derives embeddings for a list of Flickr SPR results encoded in a [gjson.Result].
func EmbeddingsForFlickrSPRArray(ctx context.Context, opts *EmbeddingsForFlickrSPROptions, photos_rsp gjson.Result) error {

	for _, ph_rsp := range photos_rsp.Array() {

		err := EmbeddingsForFlickrSPR(ctx, opts, ph_rsp)

		if err != nil {
			return err
		}
	}

	return nil
}

// EmbeddingsForFlickrSPRArray derives embeddings for a single Flickr SPR result encoded in a [gjson.Result].
func EmbeddingsForFlickrSPR(ctx context.Context, opts *EmbeddingsForFlickrSPROptions, ph_rsp gjson.Result) error {

	id_rsp := ph_rsp.Get("id")

	if !id_rsp.Exists() {
		return fmt.Errorf("SPR is missing id property")
	}

	secret_rsp := ph_rsp.Get("secret")

	if !secret_rsp.Exists() {
		return fmt.Errorf("SPR is missing secret property")
	}

	server_rsp := ph_rsp.Get("server")

	if !server_rsp.Exists() {
		return fmt.Errorf("SPR is missing server property")
	}

	id := id_rsp.String()
	secret := secret_rsp.String()
	server := server_rsp.String()

	title := ph_rsp.Get("title").String()

	logger := slog.Default()
	logger = logger.With("id", id)

	ph_url := fmt.Sprintf("https://live.staticflickr.com/%s/%s_%s.jpg", server, id, secret)
	// logger.Debug("Fetch photo", "url", ph_url)

	im_rsp, err := http.Get(ph_url)

	if err != nil {
		return fmt.Errorf("Failed to retrieve photo %s, %w", ph_url, err)
	}

	im_body, err := io.ReadAll(im_rsp.Body)
	im_rsp.Body.Close()

	if err != nil {
		return fmt.Errorf("Failed to read photo %s, %w", ph_url)
	}

	records := make([]*embeddingsdb.Record, len(opts.Models))

	for idx, model := range opts.Models {

		emb_req := &sfom_embeddings.EmbeddingsRequest{
			Model: model,
			Body:  im_body,
		}

		emb_rsp, err := opts.EmbeddingsClient.ImageEmbeddings(ctx, emb_req)

		if err != nil {
			return fmt.Errorf("Failed to derive image embeddings for %s (%s), %w", ph_url, model, err)
		}

		db_rec := &embeddingsdb.Record{
			Provider:    opts.Provider,
			DepictionId: id,
			SubjectId:   id,
			Model:       emb_rsp.Model(),
			Embeddings:  emb_rsp.Embeddings(),
			Attributes: map[string]string{
				"uri":   ph_url,
				"title": title,
			},
			Created: emb_rsp.Created(),
		}

		logger.Debug("Add record", "key", db_rec.Key())
		records[idx] = db_rec
	}

	_, err = opts.ParquetWriter.Write(records)

	if err != nil {
		return fmt.Errorf("Failed to append records to parquet writer for %s, %w", ph_url, err)
	}

	return nil
}
