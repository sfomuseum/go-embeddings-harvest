package harvest

import (
	"flag"

	"github.com/sfomuseum/go-flags/flagset"
	"github.com/sfomuseum/go-flags/multi"
)

type RunOptions struct {
	HarvesterURI           string
	EmbeddingsClientURI    string
	CacheURI               string
	CacheCheckLastmodified bool
	Params                 multi.KeyValueString
	Workers                int
	Output                 string
	Models                 multi.MultiCSVString
	Verbose                bool
	Precache               bool
}

func RunOptionsFromFlagSet(fs *flag.FlagSet) (*RunOptions, error) {

	flagset.Parse(fs)

	opts := &RunOptions{
		HarvesterURI:           harvester_uri,
		EmbeddingsClientURI:    embeddings_client_uri,
		Verbose:                verbose,
		Workers:                workers,
		Precache:               precache,
		Models:                 models,
		Output:                 output,
		Params:                 params,
		CacheURI:               cache_uri,
		CacheCheckLastmodified: cache_check_lastmod,
	}

	return opts, nil
}
