package harvest

import (
	"context"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/aaronland/go-flickr-api/client"
	"github.com/aaronland/gocloud/runtimevar"
	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/tidwall/gjson"
)

func init() {
	MustRegisterHarvester(context.Background(), "flickr", NewFlickrAPIHarvester)
}

// embeddingsForFlickrSPROptions defined configuration options to be passed to the various "EmbeddingsForFlickrSPR*" methods.
type embeddingsForFlickrSPROptions struct {
	// The name of the provider to assign to each embedding.
	Provider string
	// The list of models to derive embeddings from.
	Models []string
	// The [embeddings.Embedder[float32]] instance to derive embeddings with.
	EmbeddingsClient embeddings.Embedder[float32]
	// CacheOptions configures local or remote HTTP asset caching behavior.
	CacheOptions *http.GetWithCacheOptions
	// PreCache indicates whether assets should be downloaded into the cache without embedding them.
	PreCache bool
}

type FlickrAPIHarvester struct {
	Harvester
	flickr_client client.Client
	provider      string
	spr_path      string
}

func NewFlickrAPIHarvester(ctx context.Context, uri string) (Harvester, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	provider := u.Host

	if provider == "" {
		provider = "flickr"
	}

	spr_path := u.Path

	if spr_path == "" {
		spr_path = "photos.photo"
	}

	q := u.Query()

	flickr_client_uri := q.Get("client-uri")

	slog.Debug("Get client URI from runtimevar", "uri", flickr_client_uri)

	client_uri, err := runtimevar.StringVar(ctx, flickr_client_uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to derive Flickr client URI, %w", err)
	}

	slog.Debug("Create new Flickr API client")

	flickr_cl, err := client.NewClient(ctx, client_uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to create new Flickr client, %w", err)
	}

	h := &FlickrAPIHarvester{
		flickr_client: flickr_cl,
		provider:      provider,
		spr_path:      spr_path,
	}

	return h, nil
}

func (h *FlickrAPIHarvester) Iterate(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		logger := slog.Default()
		logger = logger.With("provider", h.provider)

		buffer := NewBuffer[*embeddingsdb.Record](100)
		defer buffer.Close()

		if opts.Verbose {
			buffer.StartStatsTicker()
		}

		args := &url.Values{}

		extras := []string{
			"owner_name",
		}

		for _, kv := range opts.Params {

			switch kv.Key() {
			case "extras":

				// Ensure that extras contains "owner_name" in order to
				// populate oembeddings attributes

				str_extras := kv.Value().(string)

				for _, e := range strings.Split(str_extras, ",") {
					e = strings.TrimSpace(e)

					if !slices.Contains(extras, e) {
						extras = append(extras, e)
					}
				}

				args.Set("extras", strings.Join(extras, ","))

			default:
				args.Set(kv.Key(), kv.Value().(string))
			}
		}

		if !args.Has("extras") {
			args.Set("extras", strings.Join(extras, ","))
		}

		if args.Has("userid") {

			userid := args.Get("userid")

			if strings.HasPrefix(userid, "nsid:") {

				username := strings.Replace(userid, "nsid:", "", 1)
				logger.Debug("Derive NSID for user", "username", username)

				args := &url.Values{}
				args.Set("method", "flickr.people.findByUsername")
				args.Set("username", username)

				rsp, err := h.flickr_client.ExecuteMethod(ctx, args)

				if err != nil {
					yield(nil, fmt.Errorf("Failed to execute method, %v", err))
					return
				}

				defer rsp.Close()

				body, err := io.ReadAll(rsp)

				if err != nil {
					yield(nil, fmt.Errorf("Failed to read response, %v", err))
					return
				}

				nsid_rsp := gjson.GetBytes(body, "user.nsid")

				if !nsid_rsp.Exists() {
					yield(nil, fmt.Errorf("Failed to derive NSID"))
					return
				}

				nsid := nsid_rsp.String()
				logger.Debug("Set userid parameter", "username", username, "nsid", nsid)

				args.Del("userid")
				args.Set("userid", nsid)
			}
		}

		logger = logger.With("args", args.Encode())

		//

		emb_opts := &embeddingsForFlickrSPROptions{
			EmbeddingsClient: opts.EmbeddingsClient,
			Models:           opts.Models,
			Provider:         h.provider,
			CacheOptions:     opts.CacheOptions,
		}

		emb_cb := func(ctx context.Context, r io.ReadSeekCloser, err error) error {

			if err != nil {
				logger.Error("Paginated response returned an error", "error", err)
				return err
			}

			defer r.Close()

			body, err := io.ReadAll(r)

			if err != nil {
				logger.Error("Failed to read API response body", "error", err)
				return fmt.Errorf("Failed to read response body, %w", err)
			}

			stat_rsp := gjson.GetBytes(body, "stat")

			if stat_rsp.String() != "ok" {

				err_code := gjson.GetBytes(body, "code").String()
				err_msg := gjson.GetBytes(body, "message").String()

				logger.Error("API did not return ok", "code", err_code, "message", err_msg)
				return fmt.Errorf("API did not return ok, %s (%s)", err_code, err_msg)
			}

			spr_parts := strings.Split(h.spr_path, ".")

			if len(spr_parts) > 0 {

				spr_root := spr_parts[0]

				page_rsp := gjson.GetBytes(body, fmt.Sprintf("%s.page", spr_root))
				pages_rsp := gjson.GetBytes(body, fmt.Sprintf("%s.pages", spr_root))
				total_rsp := gjson.GetBytes(body, fmt.Sprintf("%s.total", spr_root))

				logger.Info("process results", "page", page_rsp.Int(), "pages", pages_rsp.Int(), "total", total_rsp.Int())
			}

			// Check for error here

			photos_rsp := gjson.GetBytes(body, h.spr_path)

			if !photos_rsp.Exists() {
				logger.Error("Paginated response missing SPR path", "path", h.spr_path)
				return fmt.Errorf("Response body missing '%s' path", h.spr_path)
			}

			var ph_err error

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			wg := new(sync.WaitGroup)

			for _, ph_rsp := range photos_rsp.Array() {

				<-opts.Throttle

				select {
				case <-ctx.Done():
					break
				default:

					wg.Go(func() {

						defer func() {
							opts.Throttle <- true
						}()

						records, err := h.embeddingsForFlickrSPR(ctx, emb_opts, ph_rsp)

						if err != nil {
							logger.Error("Failed to derive embeddings for photo response", "error", err)
							ph_err = err
							return
						}

						if len(records) > 0 {
							buffer.Append(records...)
						}
					})
				}
			}

			if ph_err != nil {
				cancel()
				return ph_err
			}

			wg.Wait()

			if records := buffer.CollectAndReset(false); records != nil {
				if !yield(records, nil) {
					cancel()
					return nil
				}
			}

			return nil
		}

		logger.Debug("Execute paginated method")

		err := client.ExecuteMethodPaginatedWithClient(ctx, h.flickr_client, args, emb_cb)

		if err != nil {
			yield(nil, err)
			return
		}

		if records := buffer.CollectAndReset(true); records != nil {
			if !yield(records, nil) {
				return
			}
		}
	}
}

func (h *FlickrAPIHarvester) Close() error {
	return nil
}

// embeddingsForFlickrSPRArray derives embeddings for a single Flickr SPR result encoded in a [gjson.Result].
func (h *FlickrAPIHarvester) embeddingsForFlickrSPR(ctx context.Context, opts *embeddingsForFlickrSPROptions, ph_rsp gjson.Result) ([]*embeddingsdb.Record, error) {

	id_rsp := ph_rsp.Get("id")

	if !id_rsp.Exists() {
		return nil, fmt.Errorf("SPR is missing id property")
	}

	secret_rsp := ph_rsp.Get("secret")

	if !secret_rsp.Exists() {
		return nil, fmt.Errorf("SPR is missing secret property")
	}

	server_rsp := ph_rsp.Get("server")

	if !server_rsp.Exists() {
		return nil, fmt.Errorf("SPR is missing server property")
	}

	id := id_rsp.String()
	secret := secret_rsp.String()
	server := server_rsp.String()

	title := ph_rsp.Get("title").String()

	owner_name := ph_rsp.Get("ownername").String()
	owner_id := ph_rsp.Get("owner").String()

	logger := slog.Default()
	logger = logger.With("id", id)

	ph_url := fmt.Sprintf("https://live.staticflickr.com/%s/%s_%s.jpg", server, id, secret)

	im_body, err := http.GetBytesWithCacheAndOptions(ctx, opts.CacheOptions, ph_url)

	if err != nil {
		return nil, err
	}

	if opts.PreCache {
		return make([]*embeddingsdb.Record, 0), nil
	}

	// START OF Y U NO RETURN 'owner' in JSON SPR Flickr??!?
	// This is technically not a Flickr API bug. See discussion here:
	// https://github.com/sfomuseum/go-embeddings-harvest/issues/2
	// Pleased but amazed that photo.gne?id= still works. For now...

	subject_url := fmt.Sprintf("https://flickr.com/photo.gne?id=%s", id)

	if owner_id != "" {
		subject_url = fmt.Sprintf("https://flickr.com/photos/%s/%s", owner_id, id)
	}

	// END OF Y U NO RETURN 'owner' in JSON SPR Flickr??!?

	attrs := map[string]string{
		"type":               "image",
		"preview":            ph_url,
		"subject_url":        subject_url,
		"subject_title":      title,
		"subject_creditline": fmt.Sprintf(`Flickr member "%s"`, owner_name),
		"provider_name":      "Flickr",
		"provider_url":       "https://flickr.com",
	}

	derive_opts := &DeriveEmbeddingsRecordsOptions{
		Provider:    opts.Provider,
		DepictionId: id,
		SubjectId:   id,
		Attributes:  attrs,
		Models:      opts.Models,
		Body:        im_body,
	}

	return DeriveEmbeddingsRecords(ctx, opts.EmbeddingsClient, derive_opts)
}
