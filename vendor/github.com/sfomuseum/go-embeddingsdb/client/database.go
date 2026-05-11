package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/aaronland/go-pagination"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/database"
	"github.com/sfomuseum/go-embeddingsdb/options"
)

type DatabaseClient struct {
	Client
	db database.Database
}

func init() {
	ctx := context.Background()
	RegisterClient(ctx, "database", NewDatabaseClient)
}

// DatabaseClient will return a new [DatabaseClient] instance implementing the [Client] interface
// derived from 'uri' which is expected to take the port of:
//
//	database://?{PARAMETERS}
//
// Where {PARAMETERS} may be one or more of the following:
// * `database-uri` – A registered `sfomuseum/go-embeddingsdb/database.Database` URI for the underlying database implementation to use.
func NewDatabaseClient(ctx context.Context, uri string) (Client, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to parse URI, %w", err)
	}

	q := u.Query()

	if !q.Has("database-uri") {
		return nil, fmt.Errorf("Missing database URI, %w", err)
	}

	db_uri := q.Get("database-uri")

	db, err := database.NewDatabase(ctx, db_uri)

	if err != nil {
		return nil, err
	}

	cl := &DatabaseClient{
		db: db,
	}

	return cl, nil
}

func (cl *DatabaseClient) AddRecord(ctx context.Context, record *embeddingsdb.Record) error {
	_, err := cl.db.AddRecord(ctx, record)
	return err
}

func (cl *DatabaseClient) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest, opts ...options.Option) (*embeddingsdb.Record, error) {
	return cl.db.GetRecord(ctx, req, opts...)
}

func (cl *DatabaseClient) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest, opts ...options.Option) error {
	return cl.db.RemoveRecord(ctx, req, opts...)
}

func (cl *DatabaseClient) SimilarRecords(ctx context.Context, req *embeddingsdb.SimilarRecordsRequest, opts ...options.Option) ([]*embeddingsdb.SimilarRecord, error) {
	return cl.db.SimilarRecords(ctx, req, opts...)
}

func (cl *DatabaseClient) Models(ctx context.Context, opts ...options.Option) ([]string, error) {

	return cl.db.Models(ctx, opts...)
}

func (cl *DatabaseClient) Providers(ctx context.Context, opts ...options.Option) ([]string, error) {
	return cl.db.Providers(ctx, opts...)
}

func (cl *DatabaseClient) ListRecords(ctx context.Context, pg_opts pagination.Options, opts ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error) {
	return cl.db.ListRecords(ctx, pg_opts, opts...)
}

func (cl *DatabaseClient) SimilarRecordsById(ctx context.Context, req *embeddingsdb.SimilarRecordsByIdRequest, opts ...options.Option) ([]*embeddingsdb.SimilarRecord, error) {

	record_req := &embeddingsdb.GetRecordRequest{
		Provider:    req.Provider,
		DepictionId: req.DepictionId,
		Model:       req.Model,
	}

	record, err := cl.GetRecord(ctx, record_req)

	if err != nil {
		return nil, err
	}

	similar_req := &embeddingsdb.SimilarRecordsRequest{
		Model:      record.Model,
		Embeddings: record.Embeddings,
		Exclude: []string{
			record.DepictionId,
		},
	}

	return cl.SimilarRecords(ctx, similar_req, opts...)
}

func (cl *DatabaseClient) PaginationType(ctx context.Context, opts ...options.Option) (database.PaginationType, error) {
	return cl.db.PaginationType(ctx, opts...)
}

func (cl *DatabaseClient) Close(ctx context.Context) error {
	return cl.db.Close(ctx)
}
