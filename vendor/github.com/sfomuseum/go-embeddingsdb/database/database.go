package database

// This is all up for debate. Just testing things right now.

import (
	"context"
	"fmt"
	"iter"
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

// Database defines an interface for adding and querying vector embeddings of [embeddingsdb.Record] records.
type Database interface {
	// Add adds a [embeddingsdb.Record] instance to the underlying database implementation. Returns true or false if the addition was batched.
	AddRecord(context.Context, *embeddingsdb.Record) (bool, error)
	// The number of batched records currently waiting to be added.
	BatchedRecordsCount(context.Context) (int, error)
	// Add the pending batched records
	AddBatchedRecords(context.Context) error
	// Return the EmbeddingsDB instance record matching 'provider', 'depiction_id' and 'model'.
	GetRecord(context.Context, *embeddingsdb.GetRecordRequest) (*embeddingsdb.Record, error)
	// Remove a record from an EmbeddingsDB instance.
	RemoveRecord(context.Context, *embeddingsdb.RemoveRecordRequest) error
	// ListRecords returns a pagination list of records stored in the database.
	ListRecords(context.Context, pagination.Options, ...*ListRecordsFilter) ([]*embeddingsdb.Record, pagination.Results, error)
	// IterateRecords returns an [iter.Seq2[*embeddingsdb.Record, error]] for each record stored in the database.
	IterateRecords(context.Context) iter.Seq2[*embeddingsdb.Record, error]
	// Find similar records for a given model and record instance.
	SimilarRecords(context.Context, *embeddingsdb.SimilarRecordsRequest) ([]*embeddingsdb.SimilarRecord, error)
	// Export the contents of the database. Where and how a database is exported are left as details for specific implementations.
	Export(context.Context, string) error
	// Return the Unix timestamp of the last update to the Database instance.
	LastUpdate(context.Context) (int64, error)
	// Return the URI string used to instantiate the Database instance.
	URI() string
	// Return the unique list of models, for zero (all) or more providers, across all the embeddings.
	Models(context.Context, ...string) ([]string, error)
	// Return the unique list of providers across all the embeddings.
	Providers(context.Context) ([]string, error)
	// Close performs and terminating functions required by the database.
	Close(context.Context) error
}

// DatabaseInitializationFunc is a function defined by individual database package and used to create
// an instance of that database
type DatabaseInitializationFunc func(ctx context.Context, uri string) (Database, error)

var database_roster roster.Roster

// RegisterDatabase registers 'scheme' as a key pointing to 'init_func' in an internal lookup table
// used to create new `Database` instances by the `NewDatabase` method.
func RegisterDatabase(ctx context.Context, scheme string, init_func DatabaseInitializationFunc) error {

	err := ensureDatabaseRoster()

	if err != nil {
		return err
	}

	return database_roster.Register(ctx, scheme, init_func)
}

func ensureDatabaseRoster() error {

	if database_roster == nil {

		r, err := roster.NewDefaultRoster()

		if err != nil {
			return err
		}

		database_roster = r
	}

	return nil
}

// NewDatabase returns a new `Database` instance configured by 'uri'. The value of 'uri' is parsed
// as a `url.URL` and its scheme is used as the key for a corresponding `DatabaseInitializationFunc`
// function used to instantiate the new `Database`. It is assumed that the scheme (and initialization
// function) have been registered by the `RegisterDatabase` method.
func NewDatabase(ctx context.Context, uri string) (Database, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	scheme := u.Scheme

	i, err := database_roster.Driver(ctx, scheme)

	if err != nil {
		return nil, err
	}

	init_func := i.(DatabaseInitializationFunc)
	return init_func(ctx, uri)
}

// Schemes returns the list of schemes that have been registered.
func DatabaseSchemes() []string {

	ctx := context.Background()
	schemes := []string{}

	err := ensureDatabaseRoster()

	if err != nil {
		return schemes
	}

	for _, dr := range database_roster.Drivers(ctx) {
		scheme := fmt.Sprintf("%s://", strings.ToLower(dr))
		schemes = append(schemes, scheme)
	}

	sort.Strings(schemes)
	return schemes
}
