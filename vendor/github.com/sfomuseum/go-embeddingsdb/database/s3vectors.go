package database

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"

	aa_auth "github.com/aaronland/go-aws/v3/auth"
	"github.com/aaronland/go-pagination"
	"github.com/aaronland/go-pagination/cursor"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors/document"
	"github.com/aws/aws-sdk-go-v2/service/s3vectors/types"
	"github.com/sfomuseum/go-embeddingsdb"
	db_s3vectors "github.com/sfomuseum/go-embeddingsdb/database/s3vectors"
	"github.com/sfomuseum/go-embeddingsdb/options"
)

// S3VectorsDatabase is a concrete implementation of the
// embeddingsdb.Database interface that stores embeddings
// in an S3 Vectors bucket and index.  It optionally
// maintains a DynamoDB table for fast listing by
// provider or model.
type S3VectorsDatabase struct {
	Database
	uri             string
	bucket          string
	index           string
	dimensions      int
	client          *s3vectors.Client
	dynamodb_client *db_s3vectors.DynamoDBClient
	index_arn       string
	mu              *sync.RWMutex
	max_results     int32
	max_distance    float32
	metadata        *sync.Map
}

// S3VectorsDatabaseScheme is the URI scheme used to create
// a database backed by Amazon S3 Vectors.
const S3VectorsDatabaseScheme = "s3vectors"

func init() {

	ctx := context.Background()
	err := RegisterDatabase(ctx, S3VectorsDatabaseScheme, NewS3VectorsDatabase)

	if err != nil {
		panic(err)
	}
}

// Create a new [S3VectorsDatabase] instance for managing embeddings using the Amazon Web Services S3Vectors serice derived from 'uri' which is expected to take the form of:
//
//	s3vectors://{BUCKET_NAME}?{QUERY_PARAMETERS}
//
// Where `{BUCKET_NAME}` is the name of the S3Vectors bucket where embeddings are stored. This will be created dynamically at runtime if it does not already exist. Valid query parameters are:
//   - `index` - The name of the S3Vectors index where embeddings are stored. This will be created dynamically at runtime if it does not already exist.
//   - `region` - The AWS region where your S3Vectors bucket is stored.
//   - `credentials` - A valid `aaronland/go-aws/v3/auth` credentials string.
//   - `dimensions` – The number of dimensions for the embeddings being stored. Default is 512.
//   - `max-distance` – Update the default maximum distance when querying	for similar embeddings.	Default	is 1.0.
//   - `max-results` – Update the default number of records to return when querying for similar embeddings. Default is 10.
//   - `refresh-tags` - A boolean flag to update denormalized database properties in to index-specific "tags".
//   - `dynamodb-table` – Use a custom DynamoDB table name for storing and querying record data. Default is "s3vectors".
func NewS3VectorsDatabase(ctx context.Context, uri string) (Database, error) {

	dimensions := 512
	max_results := int32(10)
	max_distance := float32(1.0)

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	q := u.Query()

	bucket := u.Host

	if bucket == "" {
		return nil, fmt.Errorf("Missing bucket name as host")
	}

	required := []string{
		"region",
		"credentials",
		"index",
	}

	for _, k := range required {

		if !q.Has(k) {
			return nil, fmt.Errorf("Required ?%s= parameter missing", k)
		}

		if q.Get(k) == "" {
			return nil, fmt.Errorf("Required ?%s= parameter may not be empty", k)
		}
	}

	if q.Has("dimensions") {

		v, err := strconv.Atoi(q.Get("dimensions"))

		if err != nil {
			return nil, fmt.Errorf("Failed to parse ?dimension= parameter, %w", err)
		}

		dimensions = v
	}

	if q.Has("max-results") {

		v, err := strconv.Atoi(q.Get("max-results"))

		if err != nil {
			return nil, fmt.Errorf("Failed to parse ?max-results= parameter, %w", err)
		}

		max_results = int32(v)
	}

	if q.Has("max-distance") {

		v, err := strconv.ParseFloat(q.Get("max-distance"), 32)

		if err != nil {
			return nil, fmt.Errorf("Failed to parse ?max-results= parameter, %w", err)
		}

		max_distance = float32(v)
	}

	region := q.Get("region")
	creds := q.Get("credentials")
	index := q.Get("index")

	cfg_q := url.Values{}
	cfg_q.Set("region", region)
	cfg_q.Set("credentials", creds)

	cfg_u := url.URL{}
	cfg_u.Scheme = "aws"
	cfg_u.RawQuery = cfg_q.Encode()

	cfg_uri := cfg_u.String()

	cfg, err := aa_auth.NewConfig(ctx, cfg_uri)

	if err != nil {
		return nil, err
	}

	cl := s3vectors.NewFromConfig(cfg)

	index_arn, err := setupS3VectorsBucketAndIndex(ctx, cl, bucket, index, dimensions)

	if err != nil {
		return nil, err
	}

	var dynamodb_cl *db_s3vectors.DynamoDBClient

	dynamodb_table := db_s3vectors.DynamoDBTableName

	if q.Has("dynamodb-table") {

		v := q.Get("dynamodb-table")

		if v == "" {
			return nil, fmt.Errorf("?dynamodb-table= parameter may not be empty.")
		}

		dynamodb_table = v

		cfg_u, _ := url.Parse(cfg_uri)
		cfg_q := cfg_u.Query()

		if cfg_q.Has("dynamodb-table") {
			cfg_q.Del("dynamodb-table")
		}

		cfg_q.Set("dynamodb-table", dynamodb_table)

		cfg_u.RawQuery = cfg_q.Encode()
		cfg_uri = cfg_u.String()
	}

	d_cl, err := db_s3vectors.NewDynamoDBClient(ctx, cfg_uri)

	if err != nil {
		return nil, err
	}

	err = d_cl.SetupTables(ctx)

	if err != nil {
		return nil, err
	}

	dynamodb_cl = d_cl

	metadata := new(sync.Map)
	mu := new(sync.RWMutex)

	db := &S3VectorsDatabase{
		client:          cl,
		dynamodb_client: dynamodb_cl,
		index_arn:       index_arn,
		bucket:          bucket,
		index:           index,
		uri:             uri,
		dimensions:      dimensions,
		max_results:     max_results,
		max_distance:    max_distance,
		mu:              mu,
		metadata:        metadata,
	}

	return db, nil
}

// Export the contents of the database. Where and how a database is exported are left as details for specific implementations.
func (db *S3VectorsDatabase) Export(ctx context.Context, uri string, opts ...options.Option) error {
	return nil
}

// Add adds a [embeddingsdb.Record] instance to the underlying database implementation. Returns true or false if the addition was batched.
func (db *S3VectorsDatabase) AddRecord(ctx context.Context, rec *embeddingsdb.Record, opts ...options.Option) (bool, error) {

	// https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3vectors#Client.PutVectors

	attrs := rec.Attributes

	if attrs == nil {
		attrs = make(map[string]string)
	}

	attrs["x-model"] = rec.Model
	attrs["x-provider"] = rec.Provider
	attrs["x-subject-id"] = rec.SubjectId
	attrs["x-depiction-id"] = rec.DepictionId
	attrs["x-created"] = strconv.FormatInt(rec.Created, 10)

	meta := document.NewLazyDocument(attrs)

	vecs := []types.PutInputVector{
		types.PutInputVector{
			Data: &types.VectorDataMemberFloat32{
				Value: rec.Embeddings,
			},
			Key:      aws.String(rec.Key()),
			Metadata: meta,
		},
	}

	put_opts := &s3vectors.PutVectorsInput{
		VectorBucketName: aws.String(db.bucket),
		IndexName:        aws.String(db.index),
		Vectors:          vecs,
	}

	_, err := db.client.PutVectors(ctx, put_opts)

	if err != nil {
		return false, err
	}

	if db.dynamodb_client != nil {

		err := db.dynamodb_client.AddRecord(ctx, rec)

		if err != nil {
			return false, fmt.Errorf("Failed to add record to DynamoDB, %w", err)
		}
	}

	go func() {

		ctx := context.Background()

		// Don't do this more than once

		k := fmt.Sprintf("%s-%s", rec.Provider, rec.Model)

		_, exists := db.metadata.Load(k)

		if exists {
			return
		}

		// Okay, do this

		err := db.dynamodb_client.AddModelProviderMetadata(ctx, rec.Model, rec.Provider)

		if err != nil {
			slog.Error("Failed to add model/provider metadata", "provider", rec.Provider, "model", rec.Model, "error", err)
		}

		db.metadata.Store(k, true)
	}()

	return false, nil
}

// The number of batched records currently waiting to be added.
func (db *S3VectorsDatabase) BatchedRecordsCount(ctx context.Context, opts ...options.Option) (int, error) {
	return 0, nil
}

// Add the pending batched records.
func (db *S3VectorsDatabase) AddBatchedRecord(ctx context.Context, opts ...options.Option) error {
	return nil
}

// GetRecord retrieves a single Record from S3 Vectors using the
// key composed from provider, model and depiction_id.  If the
// record is not found, RecordNotFound is returned.
func (db *S3VectorsDatabase) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest, opts ...options.Option) (*embeddingsdb.Record, error) {

	// https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3vectors#Client.QueryVectors

	get_opts := &s3vectors.GetVectorsInput{
		Keys: []string{
			req.Key(),
		},
		IndexName:        aws.String(db.index),
		VectorBucketName: aws.String(db.bucket),
		ReturnMetadata:   true,
		ReturnData:       true,
	}

	rsp, err := db.client.GetVectors(ctx, get_opts)

	if err != nil {
		return nil, err
	}

	if len(rsp.Vectors) == 0 {
		return nil, RecordNotFound
	}

	vec := rsp.Vectors[0]

	return db.s3VectorToRecord(vec.Data, vec.Metadata)
}

// RemoveRecord deletes the record identified by req from the S3 Vectors
// index and, if configured, from the DynamoDB table.  Errors
// from either store are returned.
func (db *S3VectorsDatabase) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest, opts ...options.Option) error {

	// https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3vectors#Client.DeleteVectors

	del_opts := &s3vectors.DeleteVectorsInput{
		Keys: []string{
			req.Key(),
		},
		IndexName:        aws.String(db.index),
		VectorBucketName: aws.String(db.bucket),
	}

	_, err := db.client.DeleteVectors(ctx, del_opts)

	if err != nil {
		return fmt.Errorf("Failed to remove record, %w", err)
	}

	if db.dynamodb_client != nil {

		rec := &embeddingsdb.Record{
			Provider:    req.Provider,
			Model:       req.Model,
			DepictionId: req.DepictionId,
		}

		err := db.dynamodb_client.RemoveRecord(ctx, rec)

		if err != nil {
			return fmt.Errorf("Failed to remove record to DynamoDB, %w", err)
		}
	}

	// What we really is a map[string]int counter tracking models and providers
	// such that when we delete a record we decrement the relevant model and provider
	// pruning them when their respective counter reaches 0. This also means storing
	// counts in the index tags which isn't great and/or requires crawling the whole
	// index all the time. So for now we'll just live with the potential inconcistency
	// and settle for manually updating index tags when necessary.

	return nil
}

// SimilarRecords searches for embeddings similar to those in req.
// The result slice contains the matching records together with
// their similarity distance.  The search can be restricted
// by provider, model, distance and a list of depiction IDs
// to exclude via the supplied options.
func (db *S3VectorsDatabase) SimilarRecords(ctx context.Context, req *embeddingsdb.SimilarRecordsRequest, opts ...options.Option) ([]*embeddingsdb.SimilarRecord, error) {

	// https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3vectors#Client.QueryVectors
	// https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-vectors-metadata-filtering.html

	query_opts := &s3vectors.QueryVectorsInput{
		VectorBucketName: aws.String(db.bucket),
		IndexName:        aws.String(db.index),
		ReturnDistance:   true,
		ReturnMetadata:   true,
		QueryVector: &types.VectorDataMemberFloat32{
			Value: req.Embeddings,
		},
		// TopK is set below
		// Filter is set below
	}

	// https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-vectors-metadata-filtering.html

	filters := make([]map[string]any, 0)

	max_distance := options.GetMaxDistanceFromOptions(ctx, opts...)
	max_results := options.GetMaxResultsFromOptions(ctx, opts...)
	similar_provider := options.GetSimilarProviderFromOptions(ctx, opts...)

	if max_results == nil {
		max_results = &db.max_results
	}

	if max_distance == nil {
		max_distance = &db.max_distance
	}

	if similar_provider != nil {

		f := map[string]any{
			"x-provider": map[string]string{
				"$eq": *similar_provider,
			},
		}

		filters = append(filters, f)
	}

	if len(req.Exclude) > 0 {

		f := map[string]any{
			"x-depiction-id": map[string][]string{
				"$nin": req.Exclude,
			},
		}

		filters = append(filters, f)
	}

	switch len(filters) {
	case 0:
		// pass
	case 1:
		query_opts.Filter = document.NewLazyDocument(filters[0])
	default:
		f := map[string]any{
			"$and": filters,
		}

		query_opts.Filter = document.NewLazyDocument(f)
	}

	query_opts.TopK = aws.Int32(int32(*max_results))

	rsp, err := db.client.QueryVectors(ctx, query_opts)

	if err != nil {
		return nil, err
	}

	results := make([]*embeddingsdb.SimilarRecord, len(rsp.Vectors))

	for i, vec := range rsp.Vectors {

		if *max_distance > 0 && *vec.Distance > *max_distance {
			continue
		}

		meta := make(map[string]string)

		err := vec.Metadata.UnmarshalSmithyDocument(&meta)

		if err != nil {
			return nil, fmt.Errorf("Failed to unmarshal metadata")
		}

		rec := &embeddingsdb.SimilarRecord{
			Distance: *vec.Distance,
		}

		attrs := make(map[string]string)

		var provider string
		var depiction_id string
		var subject_id string

		for k, v := range meta {

			switch {
			case strings.HasPrefix(k, "x-"):
				switch k {
				case "x-provider":
					provider = v
				case "x-depiction-id":
					depiction_id = v
				case "x-subject-id":
					subject_id = v
				}
			default:
				attrs[k] = v
			}
		}

		rec.Provider = provider
		rec.DepictionId = depiction_id
		rec.SubjectId = subject_id
		rec.Attributes = attrs

		results[i] = rec
	}

	return results, nil
}

// ListRecords returns a paginated list of all records in the
// database.  When a DynamoDB client is configured the method
// falls back to using it for filtering by provider or model.
// The returned Results object contains the pagination cursors.
func (db *S3VectorsDatabase) ListRecords(ctx context.Context, pg_opts pagination.Options, opts ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error) {

	if db.dynamodb_client == nil {
		return db.listRecords(ctx, pg_opts, opts...)
	}

	// START OF please make me better somehow...

	var provider string
	var model string

	if provider == "" {

		pr := options.GetProviderFromOptions(ctx, opts...)

		if pr != nil {
			provider = *pr
		}
	}

	if provider == "" {

		v := options.GetFilterFromOptions(ctx, "provider", opts...)

		switch v.(type) {
		case string:
			provider = v.(string)
		default:
			// slog.Warn("Unexpected value for provider", "value", v)
		}
	}

	if model == "" {

		m := options.GetModelFromOptions(ctx, opts...)

		if m != nil {
			model = *m
		}
	}

	if model == "" {

		v := options.GetFilterFromOptions(ctx, "model", opts...)

		switch v.(type) {
		case string:
			model = v.(string)
		default:
			// slog.Warn("Unexpected value for model", "value", v)
		}
	}

	// END OF please make me better somehow...

	if provider == "" && model == "" {
		return db.listRecords(ctx, pg_opts, opts...)
	}

	if provider == "" {
		return db.listRecordsByModelWithDynamoDB(ctx, pg_opts, model, opts...)
	}

	return db.listRecordsByProviderWithDynamoDB(ctx, pg_opts, provider, opts...)
}

func (db *S3VectorsDatabase) listRecordsByProviderWithDynamoDB(ctx context.Context, pg_opts pagination.Options, provider string, opts ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error) {

	dynamodb_rsp, pg_rsp, err := db.dynamodb_client.ListRecordsByProvider(ctx, pg_opts, provider, opts...)

	if err != nil {
		return nil, nil, err
	}

	return db.listRecordsWithDynamoDB(ctx, dynamodb_rsp, pg_rsp)

}

func (db *S3VectorsDatabase) listRecordsByModelWithDynamoDB(ctx context.Context, pg_opts pagination.Options, model string, opts ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error) {

	dynamodb_rsp, pg_rsp, err := db.dynamodb_client.ListRecordsByModel(ctx, pg_opts, model, opts...)

	if err != nil {
		return nil, nil, err
	}

	return db.listRecordsWithDynamoDB(ctx, dynamodb_rsp, pg_rsp)
}

func (db *S3VectorsDatabase) listRecordsWithDynamoDB(ctx context.Context, dynamodb_rsp []*db_s3vectors.DynamoDBRecord, pg_rsp pagination.Results) ([]*embeddingsdb.Record, pagination.Results, error) {

	count := len(dynamodb_rsp)

	records := make([]*embeddingsdb.Record, count)

	if count == 0 {
		return records, pg_rsp, nil
	}

	type record struct {
		index  int
		record *embeddingsdb.Record
	}

	done_ch := make(chan bool)
	err_ch := make(chan error)
	rec_ch := make(chan record)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for i, r := range dynamodb_rsp {

		go func(i int, r *db_s3vectors.DynamoDBRecord) {

			defer func() {
				done_ch <- true
			}()

			req := &embeddingsdb.GetRecordRequest{
				Model:       r.Model,
				Provider:    r.Provider,
				DepictionId: r.DepictionId,
			}

			rec, err := db.GetRecord(ctx, req)

			if err != nil {
				err_ch <- fmt.Errorf("Failed to retrieve record %s, %w", req.Key(), err)
				return
			}

			rec_ch <- record{index: i, record: rec}

		}(i, r)
	}

	remaining := count

	for remaining > 0 {
		select {
		case <-done_ch:
			remaining -= 1
		case err := <-err_ch:
			return nil, nil, err
		case rec := <-rec_ch:
			records[rec.index] = rec.record
		}
	}

	return records, pg_rsp, nil
}

func (db *S3VectorsDatabase) listRecords(ctx context.Context, pg_opts pagination.Options, opts ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error) {

	// Here's the problem: The ListVectors API method does not provide
	// any way to filter things...

	// filters := GetAllFiltersFromOptions(ctx, opts...)
	// args := make([]any, len(filters))

	// https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3vectors#NewListVectorsPaginator

	list_opts := &s3vectors.ListVectorsInput{
		VectorBucketName: aws.String(db.bucket),
		IndexName:        aws.String(db.index),
		ReturnData:       true,
		ReturnMetadata:   true,
	}

	per_page := pg_opts.PerPage()
	pointer := pg_opts.Pointer()

	var prev_cursor string
	var next_cursor string

	if per_page > 0 {
		list_opts.MaxResults = aws.Int32(int32(per_page))
	}

	if pointer != nil {

		if token, ok := pointer.(string); ok && token != "" {
			prev_cursor = token
			list_opts.NextToken = aws.String(token)
		}
	}

	rsp, err := db.client.ListVectors(ctx, list_opts)

	if err != nil {
		return nil, nil, err
	}

	if rsp.NextToken != nil {
		next_cursor = *rsp.NextToken
	}

	pg, err := cursor.NewPaginationFromCursors(prev_cursor, next_cursor)

	if err != nil {
		return nil, nil, err
	}

	count := len(rsp.Vectors)
	records := make([]*embeddingsdb.Record, count)

	for i, vec := range rsp.Vectors {

		rec, err := db.s3VectorToRecord(vec.Data, vec.Metadata)

		if err != nil {
			return nil, nil, err
		}

		records[i] = rec
	}

	return records, pg, nil
}

// IterateRecords returns a [iter.Seq2[*embeddingsdb.Record, error]] that yields every
// record stored in the database.  The sequence is lazy and will continue until the context
// is cancelled or an error occurs.
func (db *S3VectorsDatabase) IterateRecords(ctx context.Context, opts ...options.Option) iter.Seq2[*embeddingsdb.Record, error] {

	return func(yield func(*embeddingsdb.Record, error) bool) {

		list_opts := &s3vectors.ListVectorsInput{
			VectorBucketName: aws.String(db.bucket),
			IndexName:        aws.String(db.index),
			ReturnData:       true,
			ReturnMetadata:   true,
		}

		pg := s3vectors.NewListVectorsPaginator(db.client, list_opts)

		for pg.HasMorePages() {

			rsp, err := pg.NextPage(ctx)

			if err != nil {
				yield(nil, err)
				return
			}

			for _, vec := range rsp.Vectors {

				rec, err := db.s3VectorToRecord(vec.Data, vec.Metadata)

				if !yield(rec, err) {
					return
				}
			}
		}
	}
}

// Return the Unix timestamp of the last update to the Database instance.
// As of this writing this always returns 0 because the cost of constantly crawling the index
// and the mechanics of denormalizing this data and then keeping in sync are too high.
func (db *S3VectorsDatabase) LastUpdate(ctx context.Context, opts ...options.Option) (int64, error) {
	return 0, nil
}

// Return the list of dimensions supported by this Database  implementation.
func (db *S3VectorsDatabase) Dimensions(ctx context.Context, opts ...options.Option) ([]int, error) {
	return []int{db.dimensions}, nil
}

// Return the unique list of models, for zero (all) or more providers, across all the embeddings.
func (db *S3VectorsDatabase) Models(ctx context.Context, opts ...options.Option) ([]string, error) {

	providers := options.GetAllProvidersFromOptions(ctx, opts...)

	switch len(providers) {
	case 0:
		return db.dynamodb_client.GetUniqueMetadataProperty(ctx, "MODELS")
	case 1:
		return db.dynamodb_client.GetModelsForProvider(ctx, providers[0])
	default:

		models_key := new(sync.Map)
		wg := new(sync.WaitGroup)

		for _, p := range providers {

			wg.Go(func() {

				models, err := db.dynamodb_client.GetModelsForProvider(ctx, p)

				if err != nil {
					slog.Error("Failed to derive models for provider", "provider", p, "error", err)
					return
				}

				for _, m := range models {
					models_key.Store(m, true)
				}
			})
		}

		wg.Wait()

		models := make([]string, 0)

		models_key.Range(func(k, v any) bool {
			models = append(models, k.(string))
			return true
		})

		return models, nil
	}
}

// Return the unique list of providers across all the embeddings.
func (db *S3VectorsDatabase) Providers(ctx context.Context, opts ...options.Option) ([]string, error) {

	models := options.GetAllModelsFromOptions(ctx, opts...)

	switch len(models) {
	case 0:
		return db.dynamodb_client.GetUniqueMetadataProperty(ctx, "PROVIDERS")
	case 1:
		return db.dynamodb_client.GetProvidersForModel(ctx, models[0])
	default:

		providers_key := new(sync.Map)
		wg := new(sync.WaitGroup)

		for _, m := range models {

			wg.Go(func() {

				providers, err := db.dynamodb_client.GetProvidersForModel(ctx, m)

				if err != nil {
					slog.Error("Failed to derive providers for model", "model", m, "error", err)
					return
				}

				for _, p := range providers {
					providers_key.Store(p, true)
				}
			})
		}

		wg.Wait()

		providers := make([]string, 0)

		providers_key.Range(func(k, v any) bool {
			providers = append(providers, k.(string))
			return true
		})

		return providers, nil
	}
}

// Return the pagination type used by the database.
func (db *S3VectorsDatabase) PaginationType(ctx context.Context, opts ...options.Option) (PaginationType, error) {
	return CursorPaginationType, nil
}

// Close performs and terminating functions required by the database.
func (db *S3VectorsDatabase) Close(ctx context.Context) error {

	return nil
}

// s3VectorToRecord converts an S3 Vectors vector and metadata
// into an embeddingsdb.Record.  The function assumes that
// the metadata contains the standard x‑ prefixed keys used
// by this implementation.
func (db *S3VectorsDatabase) s3VectorToRecord(vec_data types.VectorData, vec_meta document.Interface) (*embeddingsdb.Record, error) {

	meta := make(map[string]string)

	err := vec_meta.UnmarshalSmithyDocument(&meta)

	if err != nil {
		return nil, fmt.Errorf("Failed to unmarshal metadata")
	}

	var provider string
	var model string
	var depiction_id string
	var subject_id string
	var created int64

	attrs := make(map[string]string)

	for k, v := range meta {

		switch {
		case strings.HasPrefix(k, "x-"):

			switch k {
			case "x-provider":
				provider = v
			case "x-model":
				model = v
			case "x-depiction-id":
				depiction_id = v
			case "x-subject-id":
				subject_id = v
			case "x-created":

				created_v, err := strconv.ParseInt(v, 10, 64)

				if err != nil {
					slog.Warn("Failed to parse string created date", "date", v, "error", err)
				} else {
					created = created_v
				}

			default:
				slog.Debug("Unrecognized x- key", "k", k)
			}

		default:
			attrs[k] = v
		}
	}

	rec := &embeddingsdb.Record{
		Embeddings:  vec_data.(*types.VectorDataMemberFloat32).Value,
		Provider:    provider,
		Model:       model,
		DepictionId: depiction_id,
		SubjectId:   subject_id,
		Created:     created,
		Attributes:  attrs,
	}

	return rec, nil

}

// setupS3VectorsBucketAndIndex ensures that the bucket and index exist,
// creating them if necessary.  The function returns the index ARN.
func setupS3VectorsBucketAndIndex(ctx context.Context, cl *s3vectors.Client, bucket string, index string, dimensions int) (string, error) {

	// Maybe move in to aaronland/go-aws/s3vectors? TBD...

	logger := slog.Default()
	logger = logger.With("bucket", bucket)
	logger = logger.With("index", index)

	_, err := cl.GetVectorBucket(ctx, &s3vectors.GetVectorBucketInput{
		VectorBucketName: aws.String(bucket),
	})

	if err != nil {

		var notFound *types.NotFoundException

		if errors.As(err, &notFound) {
			logger.Debug("Bucket not found, creating")

			_, err = cl.CreateVectorBucket(ctx, &s3vectors.CreateVectorBucketInput{
				VectorBucketName: aws.String(bucket),
			})

			if err != nil {
				logger.Error("Failed to create bucket", "error", err)
				return "", err
			}
		} else {
			logger.Error("Failed to check bucket", "error", err)
			return "", err
		}
	}

	var arn string

	rsp, err := cl.GetIndex(ctx, &s3vectors.GetIndexInput{
		VectorBucketName: aws.String(bucket),
		IndexName:        aws.String(index),
	})

	if err != nil {

		var notFound *types.NotFoundException

		if errors.As(err, &notFound) {

			logger.Debug("Index not found, creating", "dimensions", dimensions)

			rsp, err := cl.CreateIndex(ctx, &s3vectors.CreateIndexInput{
				DataType:         types.DataTypeFloat32,
				VectorBucketName: aws.String(bucket),
				IndexName:        aws.String(index),
				Dimension:        aws.Int32(int32(dimensions)),
				DistanceMetric:   types.DistanceMetricCosine, // https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3vectors@v1.6.7/types#DistanceMetric
			})

			if err != nil {
				logger.Error("Failed to create index", "error", err)
				return "", err
			}

			arn = aws.ToString(rsp.IndexArn)

		} else {
			logger.Error("Failed to check index", "error", err)
			return "", err
		}
	} else {
		arn = aws.ToString(rsp.Index.IndexArn)
	}

	return arn, nil
}
