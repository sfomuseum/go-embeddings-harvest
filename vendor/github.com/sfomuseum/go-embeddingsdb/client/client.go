package client

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/aaronland/go-pagination"
	"github.com/aaronland/go-roster"
	"github.com/sfomuseum/go-embeddingsdb"
)

type ListRecordsFilter struct {
	Column string
	Value  any
	// Operator
}

// Client defines an interface for clients to interact with an embeddings database.
type Client interface {
	// Add a new record to an embeddings database.
	AddRecord(context.Context, *embeddingsdb.Record) error
	// Retrieve a specific record from an embeddings database.
	GetRecord(context.Context, *embeddingsdb.GetRecordRequest) (*embeddingsdb.Record, error)
	// Remove a record from an EmbeddingsDB instance.
	RemoveRecord(context.Context, *embeddingsdb.RemoveRecordRequest) error
	// ListRecords returns a pagination list of records stored in the database.
	ListRecords(context.Context, pagination.Options, ...*ListRecordsFilter) ([]*embeddingsdb.Record, pagination.Results, error)
	// Retrieve records with similar embeddings from an embeddings database.
	SimilarRecords(context.Context, *embeddingsdb.SimilarRecordsRequest) ([]*embeddingsdb.SimilarRecord, error)
	// Retrieve records with similar embeddings, for a specific record, from an embeddings database.
	SimilarRecordsById(context.Context, *embeddingsdb.SimilarRecordsByIdRequest) ([]*embeddingsdb.SimilarRecord, error)
	// Return the unique list of models, for zero (all) or more providers, across all the embeddings.
	Models(context.Context, ...string) ([]string, error)
	// Return the unique list of providers across all the embeddings.
	Providers(context.Context) ([]string, error)
	// Close performs and terminating functions required by the client.
	Close(context.Context) error
}

var client_roster roster.Roster

// ClientInitializationFunc is a function defined by individual client package and used to create
// an instance of that client
type ClientInitializationFunc func(ctx context.Context, uri string) (Client, error)

// RegisterClientregisters 'scheme' as a key pointing to 'init_func' in an internal lookup table
// used to create new `Client` instances by the `NewClient` method.
func RegisterClient(ctx context.Context, scheme string, init_func ClientInitializationFunc) error {

	err := ensureClientRoster()

	if err != nil {
		return err
	}

	return client_roster.Register(ctx, scheme, init_func)
}

func ensureClientRoster() error {

	if client_roster == nil {

		r, err := roster.NewDefaultRoster()

		if err != nil {
			return err
		}

		client_roster = r
	}

	return nil
}

// NewClient returns a new `Client` instance configured by 'uri'. The value of 'uri' is parsed
// as a `url.URL` and its scheme is used as the key for a corresponding `ClientInitializationFunc`
// function used to instantiate the new `Client`. It is assumed that the scheme (and initialization
// function) have been registered by the `RegisterClient` method.
func NewClient(ctx context.Context, uri string) (Client, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	scheme := u.Scheme

	i, err := client_roster.Driver(ctx, scheme)

	if err != nil {
		return nil, err
	}

	if i == nil {
		return nil, fmt.Errorf("Unregistered client")
	}

	init_func := i.(ClientInitializationFunc)

	return init_func(ctx, uri)
}

// Schemes returns the list of schemes that have been registered.
func Schemes() []string {

	ctx := context.Background()
	schemes := []string{}

	err := ensureClientRoster()

	if err != nil {
		return schemes
	}

	for _, dr := range client_roster.Drivers(ctx) {
		scheme := fmt.Sprintf("%s://", strings.ToLower(dr))
		schemes = append(schemes, scheme)
	}

	sort.Strings(schemes)
	return schemes
}
