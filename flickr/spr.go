package flickr

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/aaronland/go-flickr-api/client"
	sfom_embeddings "github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddings-harvest"
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
	// The [harvest.ParquetWriter] instance used to record data.
	Writer *harvest.ParquetWriter
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

	owner_name := ph_rsp.Get("owner_name").String()
	owner_id := ph_rsp.Get("owner").String()

	logger := slog.Default()
	logger = logger.With("id", id)

	ph_url := fmt.Sprintf("https://live.staticflickr.com/%s/%s_%s.jpg", server, id, secret)

	im_rsp, err := http.Get(ph_url)

	if err != nil {
		return fmt.Errorf("Failed to retrieve photo %s, %w", ph_url, err)
	}

	im_body, err := io.ReadAll(im_rsp.Body)
	im_rsp.Body.Close()

	if err != nil {
		return fmt.Errorf("Failed to read photo %s, %w", ph_url, err)
	}

	attrs := map[string]string{
		"type":               "image",
		"preview":            ph_url,
		"subject_url":        fmt.Sprintf("https://flickr.com/photos/%s/%s", owner_id, id),
		"subject_title":      title,
		"subject_creditline": fmt.Sprintf(`Flickr member \"%s\"`, owner_name),
		"provider_name":      "Flickr",
		"provider_url":       "https://flickr.com",
	}

	derive_opts := &harvest.DeriveEmbeddingsRecordsOptions{
		Provider:    opts.Provider,
		DepictionId: id,
		SubjectId:   id,
		Attributes:  attrs,
		Models:      opts.Models,
		Body:        im_body,
	}

	records, err := harvest.DeriveEmbeddingsRecords(ctx, opts.EmbeddingsClient, derive_opts)

	if err != nil {
		return err
	}

	if len(records) > 0 {

		_, err = opts.Writer.Write(records)

		if err != nil {
			return fmt.Errorf("Failed to append records to parquet writer for %s, %w", ph_url, err)
		}
	}

	return nil
}
