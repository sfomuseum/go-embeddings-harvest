//go:build sqlite

package database

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/aaronland/go-pagination"
	pagination_sql "github.com/aaronland/go-pagination-sql"
	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/bwmarrin/snowflake"
	sfom_sql "github.com/sfomuseum/go-database/sql"
	"github.com/sfomuseum/go-embeddingsdb"
)

//go:embed sqlite_*_schema.txt
var sqlite_schema_fs embed.FS

var snowflake_node *snowflake.Node

// SQLiteDatabase implements the [Database] interface using a SQLite database and the `sqlite-vec` extension.
type SQLiteDatabase struct {
	Database
	db_uri        string
	vec_db        *sql.DB
	vec_table     sfom_sql.Table
	records_table sfom_sql.Table
	dimensions    int
	max_distance  float32
	max_results   int32
	compression   string
}

func init() {

	ctx := context.Background()
	err := RegisterDatabase(ctx, "sqlite", NewSQLiteDatabase)

	if err != nil {
		panic(err)
	}

	n, err := snowflake.NewNode(1)

	if err != nil {
		panic(err)
	}

	snowflake_node = n
}

// NewSQLiteDatabase returns an implementation of the [Database] interface using the `sqlite-vec`
// SQLite database extension, configured using 'uri' which is expected to take the form of:
//
//	sqlite://{QUERY_PARAMETERS}
//
// Where {QUERY_PARAMETERS) may be:
// * `dsn` - A registered [database/sql.Driver] DSN string. Required.
// * `dimension` - The number of dimensions for the embeddings DB. Default is 512.
// * `max-distance` - The maximum distance between records when performing a similar records query. Default is 1.0.
// * `max-results` - The maximum number of results when	performing a similar records query. Default is 10.
// * `compression` - The type of compression to use when storing embeddings. Options are: none, quantized, matroyshka. Default is "none".
func NewSQLiteDatabase(ctx context.Context, uri string) (Database, error) {

	sqlite_vec.Auto()

	u, err := url.Parse(uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to parse URI, %w", err)
	}

	q := u.Query()

	dimensions := 512
	max_distance := float32(1.0)
	max_results := int32(10)
	compression := sqlite_vec_default_compression

	if q.Has("dimensions") {

		v, err := strconv.Atoi(q.Get("dimensions"))

		if err != nil {
			return nil, fmt.Errorf("Invalid ?dimensions= parameter, %w", err)
		}

		dimensions = v
		slog.Debug("Reassign dimensions", "value", dimensions)
	}

	if q.Has("max-distance") {

		v, err := strconv.ParseFloat(q.Get("max-distance"), 64)

		if err != nil {
			return nil, fmt.Errorf("Invalid ?max-distance= parameter, %w", err)
		}

		max_distance = float32(v)
		slog.Debug("Reassign max distance", "value", max_distance)
	}

	if q.Has("max-results") {

		v, err := strconv.Atoi(q.Get("max-results"))

		if err != nil {
			return nil, fmt.Errorf("Invalid ?max-results= parameter, %w", err)
		}

		max_results = int32(v)
		slog.Debug("Reassign max results", "value", max_results)
	}

	if q.Has("compression") {

		compression = q.Get("compression")

		if !IsValidSQLiteCompression(compression) {
			return nil, fmt.Errorf("Invalid or unsupported compression '%s'", compression)
		}

	}

	dsn := q.Get("dsn")
	vec_db, err := sql.Open("sqlite3", dsn)

	if err != nil {
		return nil, fmt.Errorf("Failed to open database connection, %w", err)
	}

	pragma := sfom_sql.DefaultSQLitePragma()
	err = sfom_sql.ConfigureSQLitePragma(ctx, vec_db, pragma)

	if err != nil {
		return nil, fmt.Errorf("Failed to config SQLite pragma, %w", err)
	}

	t_query := url.Values{}
	t_query.Set("dimensions", strconv.Itoa(dimensions))
	t_query.Set("compression", compression)

	t_uri := url.URL{}
	t_uri.Scheme = "sqlite"
	t_uri.RawQuery = t_query.Encode()

	vec_t, err := NewSQLiteVec0Table(ctx, t_uri.String())

	if err != nil {
		return nil, err
	}

	err = sfom_sql.CreateTableIfNecessary(ctx, vec_db, vec_t)

	if err != nil {
		return nil, fmt.Errorf("Failed to setup vec table, %w", err)
	}

	records_t, err := NewSQLiteRecordsTable(ctx, "sqlite://")

	if err != nil {
		return nil, err
	}

	err = sfom_sql.CreateTableIfNecessary(ctx, vec_db, records_t)

	if err != nil {
		return nil, fmt.Errorf("Failed to setup records table, %w", err)
	}

	if q.Has("max-conns") {

		v, err := strconv.Atoi(q.Get("max-conns"))

		if err != nil {
			return nil, err
		}

		vec_db.SetMaxOpenConns(v)
	}

	db := &SQLiteDatabase{
		db_uri:        uri,
		vec_db:        vec_db,
		vec_table:     vec_t,
		records_table: records_t,
		dimensions:    dimensions,
		max_distance:  max_distance,
		max_results:   max_results,
		compression:   compression,
	}

	return db, nil
}

func (db *SQLiteDatabase) Export(ctx context.Context, uri string) error {
	return nil
}

// Add adds a [embeddingsdb.Record] instance to the SQLite database.
func (db *SQLiteDatabase) AddRecord(ctx context.Context, rec *embeddingsdb.Record) error {

	id, err := db.uidForRecord(ctx, rec.Provider, rec.DepictionId, rec.Model)

	if err != nil {
		return err
	}

	enc_e, err := sqlite_vec.SerializeFloat32(rec.Embeddings)

	if err != nil {
		return err
	}

	var vec_q string

	switch db.compression {
	case sqlite_vec_quantize_compression:
		vec_q = fmt.Sprintf("INSERT OR REPLACE INTO %s (rowid, embedding) VALUES (?, vec_quantize_binary(?))", db.vec_table.Name())
	case sqlite_vec_matroyshka_compression:
		vec_q = fmt.Sprintf("INSERT OR REPLACE INTO %s (rowid, embedding) VALUES (?, vec_normalize(vec_slice(?, 0, %d)))", db.vec_table.Name(), matroyshka_dimensions)
	case sqlite_vec_default_compression:
		vec_q = fmt.Sprintf("INSERT OR REPLACE INTO %s (rowid, embedding) VALUES (?, ?)", db.vec_table.Name())
	default:
		return fmt.Errorf("Invalid or unsupported compression, '%s'", db.compression)
	}

	_, err = db.vec_db.ExecContext(ctx, vec_q, id, enc_e)

	if err != nil {
		return err
	}

	enc_attrs, err := json.Marshal(rec.Attributes)

	if err != nil {
		return err
	}

	now := time.Now()
	lastmod := now.Unix()

	records_q := fmt.Sprintf("INSERT OR REPLACE INTO %s (id, provider, depiction_id, subject_id, model, attributes, created, lastmodified) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", db.records_table.Name())

	_, err = db.vec_db.ExecContext(ctx, records_q, id, rec.Provider, rec.DepictionId, rec.SubjectId, rec.Model, string(enc_attrs), rec.Created, lastmod)

	if err != nil {
		return err
	}

	return err
}

// Return the [embeddingsdb.Record] record matching 'provider', 'depiction_id' and 'model'.
func (db *SQLiteDatabase) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest) (*embeddingsdb.Record, error) {

	id, err := db.uidForRecord(ctx, req.Provider, req.DepictionId, req.Model)

	if err != nil {
		return nil, err
	}

	records_q := fmt.Sprintf("SELECT v.embedding, r.provider, r.depiction_id, r.subject_id, r.model, r.created, r.attributes FROM %s r, %s v  WHERE r.id = v.rowid AND r.id = ?", db.records_table.Name(), db.vec_table.Name())

	row := db.vec_db.QueryRowContext(ctx, records_q, id)

	rec, err := db.inflateRecord(ctx, row)

	if err != nil {
		return nil, err
	}

	return rec, nil
}

func (db *SQLiteDatabase) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest) error {
	return fmt.Errorf("Not implemented")
}

// Find similar records for a given model and record instance.
func (db *SQLiteDatabase) SimilarRecords(ctx context.Context, req *embeddingsdb.SimilarRecordsRequest) ([]*embeddingsdb.SimilarRecord, error) {

	max_distance := db.max_distance
	max_results := db.max_results

	if req.MaxDistance != nil && *req.MaxDistance <= max_distance {
		max_distance = *req.MaxDistance
	}

	if req.MaxResults != nil && *req.MaxResults <= max_results {
		max_results = *req.MaxResults
	}

	enc_e, err := sqlite_vec.SerializeFloat32(req.Embeddings)

	if err != nil {
		return nil, err
	}

	results := make([]*embeddingsdb.SimilarRecord, 0)

	var q string

	switch db.compression {
	case sqlite_vec_quantize_compression:

		q = fmt.Sprintf("SELECT v.distance, r.provider, r.depiction_id, r.subject_id, r.attributes FROM %s r, %s v WHERE v.embedding MATCH vec_quantize_binary(?) AND r.id = v.rowid", db.records_table.Name(), db.vec_table.Name())

	case sqlite_vec_matroyshka_compression:

		q = fmt.Sprintf("SELECT v.distance, r.provider, r.depiction_id, r.subject_id, r.attributes FROM %s r, %s v WHERE v.embedding MATCH vec_normalize(vec_slice(?, 0, %d)) AND r.id = v.rowid", db.records_table.Name(), db.vec_table.Name(), matroyshka_dimensions)

	case sqlite_vec_default_compression:

		q = fmt.Sprintf("SELECT v.distance, r.provider, r.depiction_id, r.subject_id, r.attributes FROM %s r, %s v WHERE v.embedding MATCH ? AND r.id = v.rowid", db.records_table.Name(), db.vec_table.Name())

	default:
		return nil, fmt.Errorf("Invalid or unsupported compression '%s'", db.compression)
	}

	q = fmt.Sprintf("%s AND v.distance > 0", q)
	q = fmt.Sprintf("%s AND v.distance <= %f", q, max_distance)
	q = fmt.Sprintf("%s AND k=%d", q, max_results)

	rows, err := db.vec_db.QueryContext(ctx, q, enc_e)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {

		var distance float32
		var provider string
		var depiction_id string
		var subject_id string
		var str_attrs string

		err := rows.Scan(&distance, &provider, &depiction_id, &subject_id, &str_attrs)

		if err != nil {
			return nil, err
		}

		var attrs map[string]string

		err = json.Unmarshal([]byte(str_attrs), &attrs)

		if err != nil {
			return nil, err
		}

		rec := &embeddingsdb.SimilarRecord{
			Provider:    provider,
			DepictionId: depiction_id,
			SubjectId:   subject_id,
			Attributes:  attrs,
			Distance:    distance,
		}

		results = append(results, rec)
	}

	err = rows.Close()

	if err != nil {
		return nil, err
	}

	err = rows.Err()

	if err != nil {
		return nil, err
	}

	return results, nil
}

// Return the Unix timestamp of the last update to the SQLite database.
func (db *SQLiteDatabase) LastUpdate(ctx context.Context) (int64, error) {

	q := fmt.Sprintf("SELECT lastmodified FROM %s ORDER BY lastmodified DESC LIMIT 1", db.records_table.Name())

	row := db.vec_db.QueryRowContext(ctx, q)

	var lastmod int64
	err := row.Scan(&lastmod)

	switch {
	case err == sql.ErrNoRows:
		new_id := snowflake_node.Generate()
		return new_id.Int64(), nil
	case err != nil:
		return 0, err
	default:
		return lastmod, nil
	}
}

// Return the URI string used to instantiate the SQLite database.
func (db *SQLiteDatabase) URI() string {
	return db.db_uri
}

func (db *SQLiteDatabase) ListRecords(ctx context.Context, pg_opts pagination.Options, filters ...*ListRecordsFilter) ([]*embeddingsdb.Record, pagination.Results, error) {

	args := make([]any, len(filters))

	q := fmt.Sprintf("SELECT v.embedding, r.provider, r.depiction_id, r.subject_id, r.model, r.created, r. attributes FROM %s r, %s v WHERE r.id=v.rowid", db.records_table.Name(), db.vec_table.Name())

	if len(filters) > 0 {

		where := make([]string, len(filters))

		for i, f := range filters {
			where[i] = fmt.Sprintf("r.%s = ?", f.Column)
			args[i] = f.Value
		}

		q = fmt.Sprintf("%s WHERE %s", q, strings.Join(where, " AND "))
	}

	q = fmt.Sprintf("%s ORDER BY r.subject_id, r.depiction_id, r.model ASC", q)

	rsp, err := pagination_sql.QueryPaginated(db.vec_db, pg_opts, q, args...)

	if err != nil {
		return nil, nil, err
	}

	pg := rsp.Results()

	rows := rsp.Rows()
	defer rows.Close()

	records := make([]*embeddingsdb.Record, 0)

	for rows.Next() {

		r, err := db.inflateRecord(ctx, rows)

		if err != nil {
			return nil, nil, err
		}

		records = append(records, r)
	}

	err = rows.Close()

	if err != nil {
		return nil, nil, err
	}

	err = rows.Err()

	if err != nil {
		return nil, nil, err
	}

	return records, pg, nil
}

func (db *SQLiteDatabase) IterateRecords(ctx context.Context) iter.Seq2[*embeddingsdb.Record, error] {

	return func(yield func(*embeddingsdb.Record, error) bool) {

		q := fmt.Sprintf("SELECT v.embedding, r.provider, r.depiction_id, r.subject_id, r.model, r.created, r. attributes FROM %s r, %s v WHERE r.id=v.rowid ORDER BY r.created ASC", db.records_table.Name(), db.vec_table.Name())

		rows, err := db.vec_db.QueryContext(ctx, q)

		if err != nil {
			yield(nil, fmt.Errorf("Failed to execute query (%s), %w", q, err))
			return
		}

		defer rows.Close()

		for rows.Next() {

			r, err := db.inflateRecord(ctx, rows)

			if err != nil {

				if !yield(nil, err) {
					return
				}

				continue
			}

			if !yield(r, nil) {
				return
			}
		}

		err = rows.Close()

		if err != nil && !yield(nil, err) {
			return
		}

		err = rows.Err()

		if err != nil && !yield(nil, err) {
			return
		}
	}
}

// Return the unique list of models, for zero (all) or more providers, across all the embeddings.
func (db *SQLiteDatabase) Models(ctx context.Context, providers ...string) ([]string, error) {

	models := make([]string, 0)

	q := fmt.Sprintf("SELECT DISTINCT(model) FROM %s ORDER BY model ASC", db.records_table.Name())
	rows, err := db.vec_db.QueryContext(ctx, q)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {

		var model string

		err := rows.Scan(&model)

		if err != nil {
			return nil, err
		}

		models = append(models, model)
	}

	err = rows.Close()

	if err != nil {
		return nil, err
	}

	err = rows.Err()

	if err != nil {
		return nil, err
	}

	return models, nil
}

// Return the unique list of providers across all the embeddings.
func (db *SQLiteDatabase) Providers(ctx context.Context) ([]string, error) {

	providers := make([]string, 0)

	q := fmt.Sprintf("SELECT DISTINCT(provider) FROM %s ORDER BY provider ASC", db.records_table.Name())
	rows, err := db.vec_db.QueryContext(ctx, q)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	for rows.Next() {

		var provider string

		err := rows.Scan(&provider)

		if err != nil {
			return nil, err
		}

		providers = append(providers, provider)
	}

	err = rows.Close()

	if err != nil {
		return nil, err
	}

	err = rows.Err()

	if err != nil {
		return nil, err
	}

	return providers, nil
}

// Close performs and terminating functions required by the SQLite database.
func (db *SQLiteDatabase) Close(ctx context.Context) error {
	return db.vec_db.Close()
}

func (db *SQLiteDatabase) inflateRecord(ctx context.Context, rows any) (*embeddingsdb.Record, error) {

	var enc_embeddings []byte
	var provider string
	var depiction_id string
	var subject_id string
	var model string
	var created int64
	var str_attrs string

	var err error

	switch rows.(type) {
	case *sql.Row:
		err = rows.(*sql.Row).Scan(&enc_embeddings, &provider, &depiction_id, &subject_id, &model, &created, &str_attrs)
	case *sql.Rows:
		err = rows.(*sql.Rows).Scan(&enc_embeddings, &provider, &depiction_id, &subject_id, &model, &created, &str_attrs)
	default:
		return nil, fmt.Errorf("Invalid row type")
	}

	if err != nil {
		return nil, err
	}

	logger := slog.Default()
	logger = logger.With("provider", provider)
	logger = logger.With("subject_id", subject_id)
	logger = logger.With("depiction_id", depiction_id)

	var attributes map[string]string
	err = json.Unmarshal([]byte(str_attrs), &attributes)

	if err != nil {
		return nil, err
	}

	r := &embeddingsdb.Record{
		Provider:    provider,
		SubjectId:   subject_id,
		DepictionId: depiction_id,
		Model:       model,
		Attributes:  attributes,
		Created:     created,
	}

	switch db.compression {
	case sqlite_vec_quantize_compression:
		r.Embeddings = DeserializeQuantizedBinary(enc_embeddings)
	default:
		e32, err := DeserializeFloat32(enc_embeddings)

		if err != nil {
			return nil, err
		}

		r.Embeddings = e32
	}

	return r, nil
}

func (db *SQLiteDatabase) uidForRecord(ctx context.Context, provider string, depiction_id string, model string) (int64, error) {

	q := fmt.Sprintf("SELECT id FROM %s WHERE provider= ? AND depiction_id = ? AND model = ?", db.records_table.Name())

	row := db.vec_db.QueryRowContext(ctx, q, provider, depiction_id, model)

	var id int64
	err := row.Scan(&id)

	switch {
	case err == sql.ErrNoRows:
		new_id := snowflake_node.Generate()
		return new_id.Int64(), nil
	case err != nil:
		return 0, err
	default:
		return id, nil
	}
}
