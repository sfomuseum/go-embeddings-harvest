package harvest

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"strings"
	"sync"

	jw "github.com/aaronland/go-jsonl/walk"
	"github.com/aaronland/go-smithsonian-openaccess"
	"github.com/aaronland/go-smithsonian-openaccess/walk"
	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/tidwall/gjson"
	"gocloud.dev/blob"
)

func init() {
	MustRegisterHarvester(context.Background(), "si", NewSmithsonianHarvester)
}

type SmithsonianHarvester struct {
	Harvester
	units  []string
	bucket *blob.Bucket
}

func NewSmithsonianHarvester(ctx context.Context, uri string) (Harvester, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	q := u.Query()

	units := q["units"]

	bucket_uri := q.Get("bucket-uri")

	ctx, bucket, err := openaccess.OpenBucket(ctx, bucket_uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to open bucket, %w", err)
	}

	h := &SmithsonianHarvester{
		units:  units,
		bucket: bucket,
	}

	return h, nil
}

func (h *SmithsonianHarvester) Iterate(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

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

		walk_cb := func(ctx context.Context, rec *jw.WalkRecord, err error) error {

			if err != nil {
				slog.Error("Walk callback reported an error", "error", err)
				return err
			}

			<-opts.Throttle

			wg.Go(func() {

				defer func() {
					opts.Throttle <- true
				}()

				select {
				case <-ctx.Done():
					return
				default:
					// pass
				}

				logger := slog.Default()
				logger = logger.With("lineno", rec.LineNumber)

				media_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.online_media.media")
				media := media_rsp.Array()
				count_m := len(media)

				if count_m == 0 {
					return
				}

				logger.Debug("Process record", "count", count_m)

				title_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.title.content")
				subject_title := title_rsp.String()

				if subject_title == "" {
					logger.Warn("Record is missing title")
					return
				}

				subject_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.record_ID")
				subject_id := subject_rsp.String()

				if subject_id == "" {
					logger.Warn("Record is missing ID")
					return
				}

				logger = logger.With("subject", subject_id)

				link_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.record_link")
				subject_url := link_rsp.String()

				if subject_url == "" {
					// NMAH doesn't always have record_link?
					guid_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.guid")
					subject_url = guid_rsp.String()
				}

				if subject_url == "" {
					logger.Warn("Record is missing link")
					// return
				}

				unit_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.unit_code")
				unit := unit_rsp.String()

				if unit == "" {
					logger.Warn("Record is missing unit")
					return
				}

				unit = strings.ToLower(unit)
				logger = logger.With("unit", unit)

				source_rsp := gjson.GetBytes(rec.Body, "content.descriptiveNonRepeating.data_source")
				provider_name := source_rsp.String()

				if provider_name == "" {
					logger.Warn("Record is missing provider")
					return
				}

				credit_rsp := gjson.GetBytes(rec.Body, "content.freetext.creditLine.0.content")
				subject_creditline := credit_rsp.String()

				if subject_creditline == "" {
					logger.Warn("Record is missing creditline.")
					// return
				}

				for _, m := range media {

					depiction_rsp := m.Get("id")
					depiction_id := depiction_rsp.String()

					if depiction_id == "" {
						logger.Warn("Media item is missing depiction ID")
						continue
					}

					im_rsp := m.Get("content")
					im_url := im_rsp.String()

					if im_url == "" {
						logger.Warn("Media item is missing image URL (content)")
						continue
					}

					im_url = fmt.Sprintf("%s_screen", im_url)

					im_body, err := http.GetBytesWithCacheAndOptions(ctx, opts.CacheOptions, im_url)

					if err != nil {
						logger.Error("Failed to retrieve image", "url", im_url, "error", err)
						err_ch <- err
						continue
					}

					attrs := map[string]string{
						"type":               "image",
						"preview":            im_url,
						"subject_url":        subject_url,
						"subject_title":      subject_title,
						"subject_creditline": subject_creditline,
						"provider_name":      provider_name,
						"provider_url":       fmt.Sprintf("https://si.edu#%s", unit),
					}

					derive_opts := &DeriveEmbeddingsRecordsOptions{
						Provider:    unit,
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
						continue
					}

					if len(records) > 0 {
						records_ch <- records
					}

					logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
				}
			})

			return nil
		}

		for _, unit := range h.units {

			select {
			case <-ctx.Done():
				return
			default:
				// pass
			}

			uri := fmt.Sprintf("metadata/edan/%s", unit)
			slog.Debug("Query unit", "uri", uri)

			b := blob.PrefixedBucket(h.bucket, uri)

			opts := &walk.WalkOptions{
				Callback: walk_cb,
				IsBzip:   false,
				// QuerySet: qs,
			}

			err := walk.WalkBucket(ctx, opts, b)

			if err != nil {
				err_ch <- err
			}
		}

		wg.Wait()

		done_ch <- true
		close(records_ch)
		close(err_ch)
	}
}

func (h *SmithsonianHarvester) Close() error {
	return h.bucket.Close()
}
