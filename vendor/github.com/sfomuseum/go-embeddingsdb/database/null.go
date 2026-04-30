package database

import (
	"context"
	"fmt"
	"iter"

	"github.com/aaronland/go-pagination"
	"github.com/aaronland/go-pagination/countable"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/options"
)

const NullDatabaseScheme string = "null"

type NullDatabase struct {
	Database
}

func init() {

	ctx := context.Background()
	err := RegisterDatabase(ctx, NullDatabaseScheme, NewNullDatabase)

	if err != nil {
		panic(err)
	}
}

func NewNullDatabase(ctx context.Context, uri string) (Database, error) {
	db := &NullDatabase{}
	return db, nil
}

// Return the URI string used to instantiate the Database instance.
func (db *NullDatabase) URI() string {
	return fmt.Sprintf("%s://", NullDatabaseScheme)
}

// Export the contents of the database. Where and how a database is exported are left as details for specific implementations.
func (db *NullDatabase) Export(ctx context.Context, uri string, opts ...options.Option) error {
	return nil
}

// Add adds a [embeddingsdb.Record] instance to the underlying database implementation. Returns true or false if the addition was batched.
func (db *NullDatabase) AddRecord(ctx context.Context, rec *embeddingsdb.Record, opts ...options.Option) (bool, error) {
	return false, nil
}

// The number of batched records currently waiting to be added.
func (db *NullDatabase) BatchedRecordsCount(ctx context.Context, opts ...options.Option) (int, error) {
	return 0, nil
}

// Add the pending batched records.
func (db *NullDatabase) AddBatchedRecord(ctx context.Context, opts ...options.Option) error {
	return nil
}

// Return the EmbeddingsDB instance record matching 'provider', 'depiction_id' and 'model'.
func (db *NullDatabase) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest, opts ...options.Option) (*embeddingsdb.Record, error) {
	return nil, fmt.Errorf("Not found")
}

// Remove a record from an EmbeddingsDB instance.
func (db *NullDatabase) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest, opts ...options.Option) error {
	return nil
}

// Find similar records for a given model and record instance.
func (db *NullDatabase) SimilarRecords(ctx context.Context, rec *embeddingsdb.SimilarRecordsRequest, opts ...options.Option) ([]*embeddingsdb.SimilarRecord, error) {
	results := make([]*embeddingsdb.SimilarRecord, 0)
	return results, nil
}

// ListRecords returns a paginated list of records stored in the database.
func (db *NullDatabase) ListRecords(ctx context.Context, pg_opts pagination.Options, opts ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error) {

	records := make([]*embeddingsdb.Record, 0)

	pg, err := countable.NewResultsFromCountWithOptions(pg_opts, 0)

	if err != nil {
		return nil, nil, err
	}

	return records, pg, nil
}

// IterateRecords returns an [iter.Seq2[*embeddingsdb.Record, error]] for each record stored in the database.
func (db *NullDatabase) IterateRecords(ctx context.Context, opts ...options.Option) iter.Seq2[*embeddingsdb.Record, error] {
	return func(yield func(*embeddingsdb.Record, error) bool) {}
}

// Return the Unix timestamp of the last update to the Database instance.
func (db *NullDatabase) LastUpdate(ctx context.Context, opts ...options.Option) (int64, error) {
	return 0, nil
}

// Return the list of dimensions supported by this Database  implementation.
func (db *NullDatabase) Dimensions(ctx context.Context, opts ...options.Option) ([]int, error) {
	return []int{0}, nil
}

// Return the unique list of models, for zero (all) or more providers, across all the embeddings.
func (db *NullDatabase) Models(ctx context.Context, opts ...options.Option) ([]string, error) {
	models := make([]string, 0)
	return models, nil
}

// Return the unique list of providers across all the embeddings.
func (db *NullDatabase) Providers(ctx context.Context, opts ...options.Option) ([]string, error) {
	providers := make([]string, 0)
	return providers, nil
}

// Return the pagination type used by the database.
func (db *NullDatabase) PaginationType(ctx context.Context, opts ...options.Option) (PaginationType, error) {
	return NullPaginationType, nil
}

// Close performs and terminating functions required by the database.
func (db *NullDatabase) Close(ctx context.Context) error {
	return nil
}
