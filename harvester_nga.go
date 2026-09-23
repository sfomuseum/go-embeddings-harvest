package harvest

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"sync"

	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-csvdict/v2"
	"github.com/sfomuseum/go-embeddingsdb"
)

func init() {
	MustRegisterHarvester(context.Background(), "nga", NewNationalGalleryOfArtHarvester)
}

type ngaObjectInfo struct {
	Title      string
	Creditline string
}

type NationalGalleryOfArtHarvester struct {
	Harvester
	path_objects string
	path_images  string
}

func NewNationalGalleryOfArtHarvester(ctx context.Context, uri string) (Harvester, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	h := &NationalGalleryOfArtHarvester{
		path_objects: u.Path,
		path_images:  "fixme",
	}

	return h, nil
}

func (h *NationalGalleryOfArtHarvester) Iterate(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		objects_r, err := csvdict.NewReaderFromPath(h.path_objects)

		if err != nil {
			yield(nil, fmt.Errorf("Failed to create CSV reader for CMA data, %w", err))
			return
		}

		images_r, err := csvdict.NewReaderFromPath(h.path_images)

		if err != nil {
			yield(nil, fmt.Errorf("Failed to create CSV reader for images, %v", err))
			return
		}

		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// First, iterate through all the objects and capture title and creditline information
		// for inclusion below

		objects_wg := new(sync.WaitGroup)
		objects_lookup := new(sync.Map)

		for row, err := range objects_r.Iterate() {

			if err != nil {
				yield(nil, fmt.Errorf("Objects iterator yielded an error, %v", err))
				return
			}

			objects_wg.Go(func() {

				objects_lookup.Store(row["objectid"], &ngaObjectInfo{
					Title:      row["title"],
					Creditline: row["creditline"],
				})
			})
		}

		objects_wg.Wait()

		// Now fetch images

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

		for row, err := range images_r.Iterate() {

			if err != nil {
				yield(nil, fmt.Errorf("Images iterator yielded an error, %w", err))
				return
			}

			<-opts.Throttle

			wg.Go(func() {

				defer func() {
					opts.Throttle <- true
				}()

				logger := slog.Default()
				logger = logger.With("path", row["uuid"])

				depiction_id := row["uuid"]
				subject_id := row["depictstmsobjectid"]
				im_url := row["iiifthumburl"]

				logger.Debug("Fetch image", "url", im_url)

				im_body, err := http.GetBytesWithCacheAndOptions(ctx, opts.CacheOptions, im_url)

				if err != nil {
					logger.Error("Failed to retrieve image", "url", im_url, "error", err)
					err_ch <- err
					return
				}

				if opts.PreCache {
					records_ch <- make([]*embeddingsdb.Record, 0)
					return
				}

				// works: https://www.nga.gov/artworks/12198-symphony-white-no-1-white-girl
				// does not work: https://www.nga.gov/artworks/12198
				// works: purl.org/nga/collection/artobject/12198
				// see also: https://github.com/NationalGalleryOfArt/opendata/issues/19

				attrs := map[string]string{
					"type":               "image",
					"preview":            im_url,
					"subject_url":        fmt.Sprintf("https://purl.org/nga/collection/artobject/%s", subject_id),
					"subject_title":      "",
					"subject_creditline": "",
					"provider_name":      "National Gallery of Art",
					"provider_url":       "https://www.nga.gov/",
				}

				v, exists := objects_lookup.Load(subject_id)

				if !exists {
					logger.Warn("Unable to load object info", "object id", subject_id)
				} else {
					obj_info := v.(*ngaObjectInfo)
					attrs["subject_title"] = obj_info.Title
					attrs["subject_creditline"] = obj_info.Creditline
				}

				derive_opts := &DeriveEmbeddingsRecordsOptions{
					Provider:    "nga",
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

				if len(records) > 0 {
					records_ch <- records
				}

				logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
			})

			wg.Wait()

			done_ch <- true
			close(records_ch)
			close(err_ch)
		}
	}
}

func (h *NationalGalleryOfArtHarvester) Close() error {
	return nil
}
