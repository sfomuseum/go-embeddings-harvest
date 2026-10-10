package harvest

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/aaronland/go-flickr-api/client"
	"github.com/aaronland/gocloud/runtimevar"
	app "github.com/sfomuseum/go-embeddings-harvest/app/harvest"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
	"github.com/tidwall/gjson"
)

func Run(ctx context.Context) error {
	fs := DefaultFlagSet()
	return RunWithFlagSet(ctx, fs)
}

func RunWithFlagSet(ctx context.Context, fs *flag.FlagSet) error {

	flagset.Parse(fs)

	if verbose {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("Verbose logging enabled")
	}

	flickr_client_uri, err := runtimevar.StringVar(ctx, client_uri)

	if err != nil {
		return err
	}

	flickr_cl, err := client.NewClient(ctx, flickr_client_uri)

	if err != nil {
		return fmt.Errorf("Failed to create new Flickr client, %w", err)
	}

	args := &url.Values{}
	args.Set("method", "flickr.commons.getInstitutions")

	rsp, err := flickr_cl.ExecuteMethod(ctx, args)

	if err != nil {
		return err
	}

	defer rsp.Close()

	body, err := io.ReadAll(rsp)

	if err != nil {
		return fmt.Errorf("Failed to read response body, %w", err)
	}

	institutions := gjson.GetBytes(body, "institutions.institution")

	if !institutions.Exists() {
		return fmt.Errorf("Can't find institutions")
	}

	for _, i := range institutions.Array() {

		nsid_rsp := i.Get("nsid")
		urls_rsp := i.Get("urls.url")

		nsid := nsid_rsp.String()
		name := nsid

		for _, u := range urls_rsp.Array() {

			if u.Get("type").String() != "flickr" {
				continue
			}

			n := u.Get("_content").String()
			n = filepath.Base(n)
			name = strings.TrimRight(n, "/")
			break
		}

		// Make sure there are no NSID "@" symbols. For example:
		// parse \"flickr://flickr-commons-204243057%40N08?client-uri=file%3A%2F%2F%2Fusr%2Flocal%2Fsfomuseum%2Flockedbox%2Fflickr%2Fclient.txt\": invalid URL escape \"%40\""

		fq_name := fmt.Sprintf("flickr-commons-%s", strings.Replace(name, "@", "_", -1))

		harvester_q := url.Values{}
		harvester_q.Set("client-uri", client_uri)

		harvester_u := url.URL{}
		harvester_u.Scheme = "flickr"
		harvester_u.Host = fq_name
		harvester_u.RawQuery = harvester_q.Encode()

		harvester_uri := harvester_u.String()

		harvester_params := multi.KeyValueString{}
		harvester_params.Set("method=flickr.photos.search")
		harvester_params.Set(fmt.Sprintf("user_id=%s", nsid))

		fname := fmt.Sprintf("%s.parquet", fq_name)
		harvester_output := filepath.Join(output, fname)

		run_opts := &app.RunOptions{
			HarvesterURI:           harvester_uri,
			EmbeddingsClientURI:    embeddings_client_uri,
			Output:                 harvester_output,
			Params:                 harvester_params,
			Verbose:                verbose,
			Workers:                workers,
			Precache:               precache,
			Models:                 models,
			CacheURI:               cache_uri,
			CacheCheckLastmodified: cache_check_lastmod,
		}

		logger := slog.Default()
		logger = logger.With("nsid", nsid)
		logger = logger.With("name", name)

		t1 := time.Now()

		logger.Info("Harvest records")

		err = app.RunWithOptions(ctx, run_opts)

		if err != nil {
			logger.Error("Failed to harvest", "error", err)
		}

		logger.Info("Harvest complete", "time", time.Since(t1))
	}

	return nil
}
