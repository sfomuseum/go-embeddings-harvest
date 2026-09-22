package harvest

// Work in progress. Much TBD...

import (
	"context"
	"fmt"
	"iter"
	"net/url"
	"sort"
	"strings"

	"github.com/aaronland/go-roster"
	"github.com/sfomuseum/go-blobcache/http"
	"github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddingsdb"
)

type IterateOptions struct {
	EmbeddingsClient embeddings.Embedder[float32]
	CacheOptions     *http.GetWithCacheOptions
	Throttle         chan bool
	Models           []string
}

type Harvester interface {
	Iterate(context.Context, *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error]
	Close() error
}

var harvester_roster roster.Roster

// HarvesterInitializationFunc is a function defined by individual harvester package and used to create
// an instance of that harvester
type HarvesterInitializationFunc func(ctx context.Context, uri string) (Harvester, error)

func MustRegisterHarvester(ctx context.Context, scheme string, init_func HarvesterInitializationFunc) {

	err := RegisterHarvester(ctx, scheme, init_func)

	if err != nil {
		panic(err)
	}
}

// RegisterHarvester registers 'scheme' as a key pointing to 'init_func' in an internal lookup table
// used to create new `Harvester` instances by the `NewHarvester` method.
func RegisterHarvester(ctx context.Context, scheme string, init_func HarvesterInitializationFunc) error {

	err := ensureHarvesterRoster()

	if err != nil {
		return err
	}

	return harvester_roster.Register(ctx, scheme, init_func)
}

func ensureHarvesterRoster() error {

	if harvester_roster == nil {

		r, err := roster.NewDefaultRoster()

		if err != nil {
			return err
		}

		harvester_roster = r
	}

	return nil
}

// NewHarvester returns a new `Harvester` instance configured by 'uri'. The value of 'uri' is parsed
// as a `url.URL` and its scheme is used as the key for a corresponding `HarvesterInitializationFunc`
// function used to instantiate the new `Harvester`. It is assumed that the scheme (and initialization
// function) have been registered by the `RegisterHarvester` method.
func NewHarvester(ctx context.Context, uri string) (Harvester, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	scheme := u.Scheme

	i, err := harvester_roster.Driver(ctx, scheme)

	if err != nil {
		return nil, err
	}

	init_func := i.(HarvesterInitializationFunc)
	return init_func(ctx, uri)
}

// HarvesterSchemes returns the list of schemes that have been registered.
func HarvesterSchemes() []string {

	ctx := context.Background()
	schemes := []string{}

	err := ensureHarvesterRoster()

	if err != nil {
		return schemes
	}

	for _, dr := range harvester_roster.Drivers(ctx) {
		scheme := fmt.Sprintf("%s://", strings.ToLower(dr))
		schemes = append(schemes, scheme)
	}

	sort.Strings(schemes)
	return schemes
}
