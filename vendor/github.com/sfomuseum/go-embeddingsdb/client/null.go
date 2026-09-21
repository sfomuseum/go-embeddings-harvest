package client

import (
	"context"

	"github.com/aaronland/go-pagination"
	"github.com/aaronland/go-pagination/countable"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/database"
	"github.com/sfomuseum/go-embeddingsdb/options"
)

// NullClient implements the [Client] interface that does nothing.
type NullClient struct {
	Client
}

func init() {
	ctx := context.Background()
	RegisterClient(ctx, "null", NewNullClient)
}

// NewNullClient will return a new [NullClient] instance implementing the [Client] interface
// derived from 'uri' which is expected to take the port of:
//
//	null://
func NewNullClient(ctx context.Context, uri string) (Client, error) {

	e := &NullClient{}
	return e, nil
}

// AddRecord return nil (and does not add any records).
func (e *NullClient) AddRecord(ctx context.Context, record *embeddingsdb.Record) error {

	return nil
}

// GetRecord return nil (and `database.RecordNotFound`).
func (e *NullClient) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest, opts ...options.Option) (*embeddingsdb.Record, error) {

	return nil, database.RecordNotFound
}

// RemoveRecord returns nil (and does not remove anything).
func (e *NullClient) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest, opts ...options.Option) error {

	return nil
}

func (e *NullClient) ListRecords(ctx context.Context, pg_opts pagination.Options, opts ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error) {

	records := make([]*embeddingsdb.Record, 0)

	pg_rsp, err := countable.NewResultsFromCountWithOptions(pg_opts, 0)

	if err != nil {
		return nil, nil, err
	}

	return records, pg_rsp, nil
}

// SimilarRecords retrieves records with embeddings similar to those defined in 'req' (which is to say: nothing).
func (e *NullClient) SimilarRecords(ctx context.Context, req *embeddingsdb.SimilarRecordsRequest, opts ...options.Option) ([]*embeddingsdb.SimilarRecord, error) {

	return make([]*embeddingsdb.SimilarRecord, 0), nil
}

// SimilarRecordsById retrieves records with embeddings similar to those for the record matching 'provider', 'depiction_id' and 'model' (which is to say: nothing).
func (e *NullClient) SimilarRecordsById(ctx context.Context, req *embeddingsdb.SimilarRecordsByIdRequest, opts ...options.Option) ([]*embeddingsdb.SimilarRecord, error) {

	return make([]*embeddingsdb.SimilarRecord, 0), nil
}

func (e *NullClient) Models(ctx context.Context, opts ...options.Option) ([]string, error) {

	return make([]string, 0), nil
}

func (e *NullClient) Providers(ctx context.Context, opts ...options.Option) ([]string, error) {

	return make([]string, 0), nil
}

func (cl *NullClient) PaginationType(ctx context.Context, opts ...options.Option) (database.PaginationType, error) {

	return database.NullPaginationType, nil
}

func (cl *NullClient) Close(ctx context.Context) error {
	return nil
}
