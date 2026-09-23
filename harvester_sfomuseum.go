package harvest

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/url"
	"strconv"
	"sync"

	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/tidwall/gjson"
	"github.com/whosonfirst/go-reader/v2"
	"github.com/whosonfirst/go-whosonfirst/v4/feature/properties"
	"github.com/whosonfirst/go-whosonfirst/v4/iterate"
	wof_reader "github.com/whosonfirst/go-whosonfirst/v4/reader"
	"github.com/whosonfirst/go-whosonfirst/v4/uri"
)

func init() {
	MustRegisterHarvester(context.Background(), "sfomuseum", NewSFOMuseumHarvester)
}

type SFOMuseumHarvester struct {
	Harvester
	provider         string
	iterator_uri     string
	iterator_sources []string
	parent_reader    reader.Reader
}

func NewSFOMuseumHarvester(ctx context.Context, uri string) (Harvester, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to parse URI, %w", err)
	}

	q := u.Query()

	provider := u.Host

	parent_reader_uri := "https://data.whosonfirst.org"
	iterator_uri := "repo://"
	iterator_sources := q["iterator-source"]

	if q.Has("iterator-uri") {
		iterator_uri = q.Get("iterator-uri")
	}

	if q.Has("parent-reader-uri") {
		parent_reader_uri = q.Get("parent-reader-uri")
	}

	parent_r, err := reader.NewReader(ctx, parent_reader_uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to create parent reader, %w", err)
	}

	h := &SFOMuseumHarvester{
		provider:         provider,
		iterator_uri:     iterator_uri,
		iterator_sources: iterator_sources,
		parent_reader:    parent_r,
	}

	return h, nil
}

func (h *SFOMuseumHarvester) Iterate(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		switch h.provider {
		case "sfomuseum-data-media-collection", "sfomuseum-data-media":

			for records, err := range h.iterateMedia(ctx, opts) {

				if !yield(records, err) {
					return
				}
			}

		case "sfomuseum-data-socialmedia-instagram":

			for records, err := range h.iterateInstagram(ctx, opts) {

				if !yield(records, err) {
					return
				}
			}

		default:
			yield(nil, fmt.Errorf("Invalid or unsupported provider"))
			return
		}
	}
}

func (h *SFOMuseumHarvester) iterateMedia(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		iter, err := iterate.NewIterator(ctx, h.iterator_uri)

		if err != nil {
			yield(nil, fmt.Errorf("Failed to create new iterator, %w", err))
			return
		}

		records_ch := make(chan []*embeddingsdb.Record)
		err_ch := make(chan error)
		done_ch := make(chan bool)

		go func() {

			for {
				select {
				case <-ctx.Done():
					return
				case err := <-err_ch:

					if !yield(nil, err) {
						done_ch <- true
						return
					}

				case records := <-records_ch:

					if !yield(records, nil) {
						done_ch <- true
						return
					}
				}
			}
		}()

		creditlines := new(sync.Map)
		wg := new(sync.WaitGroup)

		for rec, err := range iter.Iterate(ctx, h.iterator_sources...) {

			if err != nil {
				yield(nil, err)
				return
			}

			<-opts.Throttle

			wg.Go(func() {

				defer func() {
					opts.Throttle <- true
				}()

				logger := slog.Default()
				logger = logger.With("path", rec.Path)

				id, uri_args, err := uri.ParseURI(rec.Path)

				if err != nil {
					// err_ch <- err
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
					err_ch <- err
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

				//

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

				im_body, err := http.GetBytesWithCacheAndOptions(ctx, opts.CacheOptions, im_url)

				if err != nil {
					logger.Error("Failed to retrieve image", "url", im_url, "error", err)
					return
				}

				if opts.PreCache {
					return
				}

				subject_url := fmt.Sprintf("https://collection.sfomuseum.org/id/%s", subject_id)
				subject_creditline := ""

				v, exists := creditlines.Load(parent_id)

				if exists {
					subject_creditline = v.(string)
				} else {

					parent_body, err := wof_reader.LoadBytes(ctx, h.parent_reader, parent_id)

					if err != nil {
						logger.Error("Failed to read parent body", "parent id", parent_id, "error", err)
						return
					}

					creditline_rsp := gjson.GetBytes(parent_body, "properties.sfomuseum:creditline")

					if !creditline_rsp.Exists() {
						logger.Error("Parent record is missing creditline", "parent id", parent_id)
						return
					}

					subject_creditline = creditline_rsp.String()
					creditlines.Store(parent_id, subject_creditline)
				}

				attrs := map[string]string{
					"type":               "image",
					"preview":            im_url,
					"subject_url":        subject_url,
					"subject_title":      name,
					"subject_creditline": subject_creditline,
					"provider_name":      "SFO Museum",
					"provider_url":       "https://collection.sfomuseum.org",
				}

				derive_opts := &DeriveEmbeddingsRecordsOptions{
					Provider:    h.provider,
					DepictionId: depiction_id,
					SubjectId:   subject_id,
					Attributes:  attrs,
					Models:      opts.Models,
					Body:        im_body,
				}

				records, err := DeriveEmbeddingsRecords(ctx, opts.EmbeddingsClient, derive_opts)

				if err != nil {
					logger.Error("Failed to derive embeddings records", "error", err)
					err_ch <- err
					return
				}

				if len(records) == 0 {
					records_ch <- records
				}

				logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
			})

		}

		wg.Wait()

		done_ch <- true
		close(records_ch)
		close(err_ch)
	}
}

func (h *SFOMuseumHarvester) iterateInstagram(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		iter, err := iterate.NewIterator(ctx, h.iterator_uri)

		if err != nil {
			yield(nil, fmt.Errorf("Failed to create new iterator, %w", err))
			return
		}

		records_ch := make(chan []*embeddingsdb.Record)
		err_ch := make(chan error)
		done_ch := make(chan bool)

		go func() {

			for {
				select {
				case <-ctx.Done():
					return
				case err := <-err_ch:

					if !yield(nil, err) {
						done_ch <- true
						return
					}

				case records := <-records_ch:

					if !yield(records, nil) {
						done_ch <- true
						return
					}
				}
			}
		}()

		wg := new(sync.WaitGroup)

		for rec, err := range iter.Iterate(ctx, h.iterator_sources...) {

			if err != nil {
				yield(nil, err)
				return
			}

			<-opts.Throttle

			wg.Go(func() {

				defer func() {
					opts.Throttle <- true
				}()

				logger := slog.Default()
				logger = logger.With("path", rec.Path)

				body, err := rec.ReadAllAndClose()

				if err != nil {
					logger.Error("Failed to read record body", "error", err)
					return
				}

				id, err := properties.Id(body)

				if err != nil {
					logger.Error("Failed to derive ID", "error", err)
					return
				}

				logger = logger.With("id", id)

				name, err := properties.Name(body)

				if err != nil {
					logger.Error("Failed to derive name", "error", err)
					return
				}

				id_rsp := gjson.GetBytes(body, "properties.instagram:post.media_id")

				if !id_rsp.Exists() {
					logger.Error("Failed to derive media ID")
					return
				}

				media_id := id_rsp.String()
				logger = logger.With("media id", media_id)

				taken_rsp := gjson.GetBytes(body, "properties.instagram:post.taken_at")

				if !taken_rsp.Exists() {
					logger.Error("Failed to derive taken at date")
					return
				}

				taken_at := taken_rsp.String()

				depiction_id := strconv.FormatInt(id, 10)
				subject_id := media_id

				// Update to use https://github.com/sfomuseum/go-sfomuseum-instagram-publish/blob/main/secret/secret.go
				ig_secret := h.instagramSecret(media_id)

				im_url := fmt.Sprintf("https://static.sfomuseum.org/media/instagram/%s/%s_%s_n.jpg", media_id, media_id, ig_secret)
				logger.Debug("Fetch image", "url", im_url)

				im_body, err := http.GetBytesWithCacheAndOptions(ctx, opts.CacheOptions, im_url)

				if err != nil {
					logger.Error("Failed to retrieve image", "url", im_url, "error", err)
					// err_ch <- err
					return
				}

				if opts.PreCache {
					return
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

				derive_opts := &DeriveEmbeddingsRecordsOptions{
					Provider:    h.provider,
					DepictionId: depiction_id,
					SubjectId:   subject_id,
					Attributes:  attrs,
					Models:      opts.Models,
					Body:        im_body,
				}

				records, err := DeriveEmbeddingsRecords(ctx, opts.EmbeddingsClient, derive_opts)

				if err != nil {
					logger.Error("Failed to derive embeddings records", "error", err)
					// err_ch <- err
					return
				}

				if len(records) == 0 {
					records_ch <- records
				}

				logger.Debug("Wrote embeddings for instagram image", "url", im_url)
			})
		}
		
		wg.Wait()

		done_ch <- true
		close(records_ch)
		close(err_ch)
	}
}

func (h *SFOMuseumHarvester) Close() error {
	return nil
}

func (h *SFOMuseumHarvester) instagramSecret(media_id string) string {

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
