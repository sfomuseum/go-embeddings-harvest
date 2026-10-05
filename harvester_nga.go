package harvest

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"maps"
	"net/url"
	"sync"

	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-csvdict/v2"
	"github.com/sfomuseum/go-embeddingsdb"
)

func init() {
	MustRegisterHarvester(context.Background(), "nga", NewNationalGalleryOfArtHarvester)
}

// ngaObjectInfo encapsulates fundamental provenance descriptors for tracking
// and cross-referencing parent artwork datasets during secondary image iterations.
type ngaObjectInfo struct {
	// Title represents the primary descriptive designation of the artwork.
	Title string
	// Creditline documents the formal acquisition or donor attribution line.
	Creditline string
}

// NationalGalleryOfArtHarvester implements the Harvester interface to parse core object listings
// and relational published image records from the National Gallery of Art's OpenData dataset.
type NationalGalleryOfArtHarvester struct {
	Harvester
	// path_objects defines the filesystem path targeting the main NGA objects.csv file.
	path_objects string
	// path_images defines the filesystem path targeting the corresponding published_images.csv file.
	path_images string
}

// NewNationalGalleryOfArtHarvester instantiates and returns a new Harvester for the
// National Gallery of Art dataset. It requires an absolute file path mapping the objects dataset
// and an explicit "images" query parameter mapping the location of published asset lists.
//
// Example:
//
//	nga:///usr/local/data/nga/opendata/data/objects.csv?images=/usr/local/data/nga/opendata/data/published_images.csv
func NewNationalGalleryOfArtHarvester(ctx context.Context, uri string) (Harvester, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	q := u.Query()

	if !q.Has("images") {
		return nil, fmt.Errorf("Missing ?images= parameter")
	}

	h := &NationalGalleryOfArtHarvester{
		path_objects: u.Path,
		path_images:  q.Get("images"),
	}

	return h, nil
}

// Iterate processes the NGA open data ecosystem via a two-pass workflow. First, it maps core artwork
// traits out to a fast lookup directory on parallel threads. Next, it reads the separate published
// images catalogue, pairs related descriptors by reference key matching, downloads media, and yields
// chunks of mapped embeddingsdb.Record vector blocks.
func (h *NationalGalleryOfArtHarvester) Iterate(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		objects_r, err := csvdict.NewReaderFromPath(h.path_objects)

		if err != nil {
			yield(nil, fmt.Errorf("Failed to create CSV reader for NGA data, %w", err))
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

		// This is what we use to process records concurrently, capturing and
		// yielding records without spilling outside of the main loop which causes
		// all kinds of iterator/yield pain.

		buffer := NewBuffer[*embeddingsdb.Record](100)
		defer buffer.Close()

		if opts.Verbose {
			buffer.StartStatsTicker()
		}

		wg := new(sync.WaitGroup)

		for iter_row, err := range images_r.Iterate() {

			if err != nil {
				yield(nil, fmt.Errorf("Images iterator yielded an error, %w", err))
				return
			}

			<-opts.Throttle

			row := maps.Clone(iter_row)

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
					return
				}

				if opts.PreCache {
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
					return
				}

				if len(records) > 0 {
					buffer.Append(records...)
				}

				logger.Debug("Wrote embeddings for exhibition image", "url", im_url)
			})

			if records := buffer.CollectAndReset(false); records != nil {
				if !yield(records, nil) {
					cancel()
					return
				}
			}
		}

		wg.Wait()

		if records := buffer.CollectAndReset(true); records != nil {
			if !yield(records, nil) {
				cancel()
				return
			}
		}
	}
}

// Close safely shuts down internal network connections or resource hooks managed
// by the NationalGalleryOfArtHarvester. It operates as an interface-compliant no-op.
func (h *NationalGalleryOfArtHarvester) Close() error {
	return nil
}
