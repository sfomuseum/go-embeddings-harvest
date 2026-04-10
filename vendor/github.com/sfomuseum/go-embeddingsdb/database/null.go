package database

import (
	"context"
	"fmt"
	"iter"

	"github.com/aaronland/go-pagination"
	"github.com/aaronland/go-pagination/countable"
	"github.com/sfomuseum/go-embeddingsdb"
)

type NullDatabase struct {
	Database
}

func init() {

	ctx := context.Background()
	err := RegisterDatabase(ctx, "null", NewNullDatabase)

	if err != nil {
		panic(err)
	}
}

func NewNullDatabase(ctx context.Context, uri string) (Database, error) {
	db := &NullDatabase{}
	return db, nil
}

func (db *NullDatabase) Export(ctx context.Context, uri string) error {
	return nil
}

func (db *NullDatabase) AddRecord(ctx context.Context, rec *embeddingsdb.Record) error {
	return nil
}

func (db *NullDatabase) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest) (*embeddingsdb.Record, error) {
	return nil, fmt.Errorf("Not found")
}

func (db *NullDatabase) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest) error {
	return nil
}

func (db *NullDatabase) SimilarRecords(ctx context.Context, rec *embeddingsdb.SimilarRecordsRequest) ([]*embeddingsdb.SimilarRecord, error) {
	results := make([]*embeddingsdb.SimilarRecord, 0)
	return results, nil
}

func (db *NullDatabase) ListRecords(ctx context.Context, opts pagination.Options, filters ...*ListRecordsFilter) ([]*embeddingsdb.Record, pagination.Results, error) {

	records := make([]*embeddingsdb.Record, 0)

	pg, err := countable.NewResultsFromCountWithOptions(opts, 0)

	if err != nil {
		return nil, nil, err
	}

	return records, pg, nil
}

func (db *NullDatabase) IterateRecords(ctx context.Context) iter.Seq2[*embeddingsdb.Record, error] {
	return func(yield func(*embeddingsdb.Record, error) bool) {}
}

func (db *NullDatabase) LastUpdate(ctx context.Context) (int64, error) {
	return 0, nil
}

func (db *NullDatabase) URI() string {
	return "null://"
}

func (db *NullDatabase) Models(ctx context.Context, providers ...string) ([]string, error) {
	models := make([]string, 0)
	return models, nil
}

func (db *NullDatabase) Providers(ctx context.Context) ([]string, error) {
	providers := make([]string, 0)
	return providers, nil
}

func (db *NullDatabase) Close(ctx context.Context) error {
	return nil
}
