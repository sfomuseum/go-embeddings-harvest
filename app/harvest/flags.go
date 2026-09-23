package harvest

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sfomuseum/go-embeddings-harvest"
	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
)

var harvester_uri string
var embeddings_client_uri string
var cache_uri string
var cache_check_lastmod bool

var workers int
var output string
var verbose bool
var precache bool

var models multi.MultiCSVString

func DefaultFlagSet() *flag.FlagSet {

	fs := flagset.NewFlagSet("cma")

	str_schemes := strings.Join(harvest.HarvesterSchemes(), ", ")
	harvester_desc := fmt.Sprintf("A registered sfomuseum/go-embessings-harvest.Harvester URI. Valid options are: %s", str_schemes)

	fs.StringVar(&harvester_uri, "harvester-uri", "null://", harvester_desc)
	fs.IntVar(&workers, "workers", 5, "The number of workers to use to fetch images (and derive embeddings) concurrently")
	fs.BoolVar(&precache, "precache", false, "Fetch images from source and store in (blob) cache without generating embeddings. If true this flag will reassign -output to /dev/null.")

	fs.Var(&models, "model", "One or more models to derive embeddings for. This may also be a comma-separated list.")

	fs.StringVar(&output, "output", "/dev/null", "The path where Parquet-encoded data should be written. If \"-\" then data will be written to STDOUT.")
	fs.StringVar(&embeddings_client_uri, "embeddings-client-uri", "mobileclip://?client-uri=grpc://localhost:8080", "A registered sfomuseum/go-embeddingsdb/client.Client URI.")

	fs.StringVar(&cache_uri, "cache-uri", "null://", "A register gocloud.dev/blob.Bucket URI to use for caching images. If null:// then no images will be cached.")
	fs.BoolVar(&cache_check_lastmod, "cache-check-lastmod", false, "A boolean value to indicate whether the last modified date of an object to harvest should be compared against the local cache.")

	fs.BoolVar(&verbose, "verbose", false, "Enable verbose (debug) logging.")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Generate Parquet-encoded embeddings from the Cleveland Museum of Art (CMA) open data release.\n")
		fmt.Fprintf(os.Stderr, "Usage:\n\t%s [options]", os.Args[0])
		fmt.Fprintf(os.Stderr, "Valid options are:\n")
		fs.PrintDefaults()
	}

	return fs
}
