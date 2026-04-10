package client

import (
	"context"
	"fmt"
	"net/url"

	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/database"
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
	return cl.db.AddRecord(ctx, record)
}

func (cl *DatabaseClient) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest) (*embeddingsdb.Record, error) {
	return cl.db.GetRecord(ctx, req)
}

func (cl *DatabaseClient) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest) error {
	return cl.db.RemoveRecord(ctx, req)
}

func (cl *DatabaseClient) SimilarRecords(ctx context.Context, req *embeddingsdb.SimilarRecordsRequest) ([]*embeddingsdb.SimilarRecord, error) {
	return cl.db.SimilarRecords(ctx, req)
}

func (cl *DatabaseClient) Models(ctx context.Context, providers ...string) ([]string, error) {
	return cl.db.Models(ctx, providers...)
}

func (cl *DatabaseClient) Providers(ctx context.Context) ([]string, error) {
	return cl.db.Providers(ctx)
}

func (cl *DatabaseClient) SimilarRecordsById(ctx context.Context, req *embeddingsdb.SimilarRecordsByIdRequest) ([]*embeddingsdb.SimilarRecord, error) {

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
		SimilarProvider: req.SimilarProvider,
		MaxDistance:     req.MaxDistance,
		MaxResults:      req.MaxResults,
	}

	return cl.SimilarRecords(ctx, similar_req)
}
