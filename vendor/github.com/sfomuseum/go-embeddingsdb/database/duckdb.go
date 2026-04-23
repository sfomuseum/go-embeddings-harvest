package database

// https://duckdb.org/2024/05/03/vector-similarity-search-vss.html
// https://duckdb.org/docs/api/go.html
// https://pkg.go.dev/github.com/marcboeker/go-duckdb
// https://github.com/marcboeker/go-duckdb/tree/main?tab=readme-ov-file#vendoring

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"

	"github.com/aaronland/go-pagination"
	pagination_sql "github.com/aaronland/go-pagination-sql"
	"github.com/sfomuseum/go-embeddingsdb"
)

type DuckDBDatabase struct {
	Database
	// The underlying SQLite database used to store and query embeddings.
	vec_db *sql.DB
	// ...
	db_uri string
	// The number of dimensions for embeddings
	dimensions int
	// The maximum number of results for queries
	max_results int32
	// ..
	max_distance float32
}

func init() {

	ctx := context.Background()
	err := RegisterDatabase(ctx, "duckdb", NewDuckDBDatabase)

	if err != nil {
		panic(err)
	}
}

// Create a new [DuckDBDatabase] instance for managing embeddings using the DuckDB database and VSS extension derived from 'uri' which is expected to take the form of:
//
//	duckdb://{PATH}?{QUERY_PARAMETERS}
//
// Valid query parameters are:
// * `dimensions` – The number of dimensions for the embeddings being stored. Default is 512.
// * `max-distance` – Update the default maximum distance when querying	for similar embeddings.	Default	is 1.0.
// * `max-results` – Update the default number of records to return when querying for similar embeddings. Default is 10.
func NewDuckDBDatabase(ctx context.Context, uri string) (Database, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to parse URI, %w", err)
	}

	q := u.Query()

	dimensions := 512
	max_distance := float32(1.0)
	max_results := int32(10)

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

	vec_db, err := sql.Open("duckdb", "")

	if err != nil {
		return nil, fmt.Errorf("Failed to open database connection, %w", err)
	}

	setup_opts := &SetupDuckDBDatabaseOptions{
		Dimensions: dimensions,
	}

	if u.Path != "" {

		abs_path, err := filepath.Abs(u.Path)

		if err != nil {
			return nil, fmt.Errorf("Failed to derive absolute path for database, %w", err)
		}

		setup_opts.DatabasePath = abs_path
	}

	err = SetupDuckDBDatabase(ctx, vec_db, setup_opts)

	if err != nil {
		return nil, fmt.Errorf("Failed to setup database, %w", err)
	}

	if q.Has("max-conns") {

		v, err := strconv.Atoi(q.Get("max-conns"))

		if err != nil {
			return nil, err
		}

		vec_db.SetMaxOpenConns(v)
	}

	db := &DuckDBDatabase{
		db_uri:       uri,
		vec_db:       vec_db,
		dimensions:   dimensions,
		max_distance: max_distance,
		max_results:  max_results,
	}

	return db, nil
}

func (db *DuckDBDatabase) Export(ctx context.Context, uri string) error {

	// There does not appear to be any way to use query placeholders for this.
	// Note: Export directory does not need to exist before calling EXPORT...

	q := fmt.Sprintf("EXPORT DATABASE '%s'", uri)
	_, err := db.vec_db.ExecContext(ctx, q)
	return err
}

func (db *DuckDBDatabase) AddRecord(ctx context.Context, rec *embeddingsdb.Record) (bool, error) {

	provider := rec.Provider
	depiction_id := rec.DepictionId
	subject_id := rec.SubjectId
	model := rec.Model
	created := rec.Created

	now := time.Now()
	lastmod := now.Unix()

	embeddings, err := json.Marshal(rec.Embeddings)

	if err != nil {
		return false, fmt.Errorf("Failed to marshal embeddings for ID %s, %w", depiction_id, err)
	}

	attributes, err := json.Marshal(rec.Attributes)

	if err != nil {
		return false, fmt.Errorf("Failed to marshal attributes for ID %s, %w", depiction_id, err)
	}

	q := "INSERT OR REPLACE INTO embeddings (provider, depiction_id, subject_id, model, attributes, vec, created, lastmodified) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"

	_, err = db.vec_db.ExecContext(ctx, q, provider, depiction_id, subject_id, model, string(attributes), string(embeddings), created, lastmod)

	if err != nil {
		return false, fmt.Errorf("Failed to add embeddings for %s, %w", depiction_id, err)
	}

	return false, nil
}

func (db *DuckDBDatabase) BatchedRecordsCount(ctx context.Context) (int, error) {
	return 0, nil
}

func (db *DuckDBDatabase) AddBatchedRecord(ctx context.Context) error {
	return nil
}

func (db *DuckDBDatabase) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest) (*embeddingsdb.Record, error) {

	q := "SELECT provider, depiction_id, subject_id, model, vec, created, attributes FROM embeddings WHERE provider = ? AND depiction_id = ? AND model = ?"

	row := db.vec_db.QueryRowContext(ctx, q, req.Provider, req.DepictionId, req.Model)
	return InflateDuckDBRecord(ctx, row)
}

func (db *DuckDBDatabase) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest) error {

	q := "DELETE FROM embeddings WHERE provider = ? AND depiction_id = ? AND model = ?"

	_, err := db.vec_db.ExecContext(ctx, q, req.Provider, req.DepictionId, req.Model)
	return err
}

func (db *DuckDBDatabase) SimilarRecords(ctx context.Context, req *embeddingsdb.SimilarRecordsRequest) ([]*embeddingsdb.SimilarRecord, error) {

	results := make([]*embeddingsdb.SimilarRecord, 0)

	embeddings, err := json.Marshal(req.Embeddings)

	if err != nil {
		return nil, fmt.Errorf("Failed to serialize query, %w", err)
	}

	max_results := db.max_results
	max_distance := db.max_distance

	if req.MaxResults != nil {
		max_results = *req.MaxResults
	}

	if req.MaxDistance != nil {
		max_distance = *req.MaxDistance
	}

	conditions := make([]string, 0)

	args := []any{
		string(embeddings),
	}

	if req.SimilarProvider != nil {
		conditions = append(conditions, "provider = ?")
		args = append(args, &req.SimilarProvider)
	}

	count_exclude := len(req.Exclude)

	if count_exclude > 0 {

		placeholders := make([]string, count_exclude)

		for i := 0; i < count_exclude; i++ {
			args = append(args, req.Exclude[i])
			placeholders[i] = "?"
		}

		conditions = append(conditions, fmt.Sprintf("depiction_id NOT IN (%s)", strings.Join(placeholders, ",")))
	}

	conditions = append(conditions, "model == ?")
	args = append(args, req.Model)

	conditions = append(conditions, "distance > 0")

	conditions = append(conditions, "distance <= ?")
	args = append(args, max_distance)

	str_conditions := strings.Join(conditions, " AND ")

	q := fmt.Sprintf(`SELECT provider, depiction_id, subject_id, attributes, array_distance(vec, ?::FLOAT[%d]) AS distance
			  FROM embeddings WHERE %s ORDER BY distance ASC LIMIT %d`,
		db.dimensions, str_conditions, max_results)

	rows, err := db.vec_db.QueryContext(ctx, q, args...)

	if err != nil {
		return nil, fmt.Errorf("Failed to execute query (%s), %w", q, err)
	}

	for rows.Next() {

		var provider string
		var depiction_id string
		var subject_id string
		var placeholder_attributes string
		var distance float64

		err = rows.Scan(&provider, &depiction_id, &subject_id, &placeholder_attributes, &distance)

		if err != nil {
			return nil, fmt.Errorf("Failed to scan row, %w", err)
		}

		var attributes map[string]string

		err = json.Unmarshal([]byte(placeholder_attributes), &attributes)

		if err != nil {
			return nil, err
		}

		r := &embeddingsdb.SimilarRecord{
			Provider:    provider,
			SubjectId:   subject_id,
			DepictionId: depiction_id,
			Attributes:  attributes,
			Distance:    float32(distance),
		}

		results = append(results, r)
	}

	return results, nil
}

func (db *DuckDBDatabase) LastUpdate(ctx context.Context) (int64, error) {

	q := "SELECT lastmodified FROM embeddings ORDER BY lastmodified DESC LIMIT 1"

	row := db.vec_db.QueryRowContext(ctx, q)

	var lastmod int64

	err := row.Scan(&lastmod)

	if err != nil {
		return 0, err
	}

	return lastmod, nil
}

func (db *DuckDBDatabase) URI() string {
	return db.db_uri
}

func (db *DuckDBDatabase) ListRecords(ctx context.Context, pg_opts pagination.Options, filters ...*ListRecordsFilter) ([]*embeddingsdb.Record, pagination.Results, error) {

	q := "SELECT provider, depiction_id, subject_id, model, vec, created, attributes FROM embeddings"
	args := make([]any, len(filters))

	if len(filters) > 0 {

		where := make([]string, len(filters))

		for i, f := range filters {
			where[i] = fmt.Sprintf("%s = ?", f.Column)
			args[i] = f.Value
		}

		q = fmt.Sprintf("%s WHERE %s", q, strings.Join(where, " AND "))
	}

	q = fmt.Sprintf("%s ORDER BY subject_id ASC, depiction_id ASC, model ASC", q)

	rsp, err := pagination_sql.QueryPaginated(db.vec_db, pg_opts, q, args...)

	if err != nil {
		return nil, nil, err
	}

	pg := rsp.Results()

	rows := rsp.Rows()
	defer rows.Close()

	records := make([]*embeddingsdb.Record, 0)

	for rows.Next() {

		r, err := InflateDuckDBRecord(ctx, rows)

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

func (db *DuckDBDatabase) IterateRecords(ctx context.Context) iter.Seq2[*embeddingsdb.Record, error] {

	return func(yield func(*embeddingsdb.Record, error) bool) {

		q := "SELECT provider, depiction_id, subject_id, model, vec, created, attributes FROM embeddings ORDER BY created ASC"

		rows, err := db.vec_db.QueryContext(ctx, q)

		if err != nil {
			yield(nil, fmt.Errorf("Failed to execute query (%s), %w", q, err))
			return
		}

		defer rows.Close()

		for rows.Next() {

			r, err := InflateDuckDBRecord(ctx, rows)

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

func (db *DuckDBDatabase) Models(ctx context.Context, providers ...string) ([]string, error) {

	logger := slog.Default()
	count_providers := len(providers)

	q := "SELECT DISTINCT(model) AS model FROM embeddings WHERE model != ''"
	args := make([]any, 0)

	if count_providers > 0 {

		in := make([]string, count_providers)
		args = make([]any, count_providers)

		for i, pr := range providers {
			in[i] = "?"
			args[i] = pr
		}

		q = fmt.Sprintf("%s AND provider IN (%s)", q, strings.Join(in, ","))
	}

	q = fmt.Sprintf("%s ORDER by model ASC", q)

	rows, err := db.vec_db.QueryContext(ctx, q, args...)

	if err != nil {
		logger.Error("Failed to query models", "q", q, "error", err)
		return nil, fmt.Errorf("Failed to query models, %w", err)
	}

	defer rows.Close()

	models := make([]string, 0)

	for rows.Next() {

		var model string
		err := rows.Scan(&model)

		if err != nil {
			return nil, fmt.Errorf("Failed to scan model row, %w", err)
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

func (db *DuckDBDatabase) Providers(ctx context.Context) ([]string, error) {

	q := "SELECT DISTINCT(provider) AS provider FROM embeddings WHERE provider != '' ORDER BY provider ASC"

	rows, err := db.vec_db.QueryContext(ctx, q)

	if err != nil {
		return nil, fmt.Errorf("Failed to query providers, %w", err)
	}

	defer rows.Close()

	providers := make([]string, 0)

	for rows.Next() {

		var provider string
		err := rows.Scan(&provider)

		if err != nil {
			return nil, fmt.Errorf("Failed to scan provider row, %w", err)
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

func (db *DuckDBDatabase) Close(ctx context.Context) error {
	return db.vec_db.Close()
}

func InflateDuckDBRecord(ctx context.Context, rows any) (*embeddingsdb.Record, error) {

	var provider string
	var depiction_id string
	var subject_id string
	var model string
	var placeholder_embeddings []interface{}
	var created int64
	var str_attrs string

	var err error

	switch rows.(type) {
	case *sql.Row:
		err = rows.(*sql.Row).Scan(&provider, &depiction_id, &subject_id, &model, &placeholder_embeddings, &created, &str_attrs)
	case *sql.Rows:
		err = rows.(*sql.Rows).Scan(&provider, &depiction_id, &subject_id, &model, &placeholder_embeddings, &created, &str_attrs)
	default:
		return nil, fmt.Errorf("Invalid type")
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

		return nil, fmt.Errorf("Failed to unmarshal attributes, %w", err)
	}

	// Thanks for making things weird, DuckDB...

	embeddings := make([]float32, len(placeholder_embeddings))

	for idx, v := range placeholder_embeddings {
		embeddings[idx] = v.(float32)
	}

	r := &embeddingsdb.Record{
		Provider:    provider,
		SubjectId:   subject_id,
		DepictionId: depiction_id,
		Model:       model,
		Embeddings:  embeddings,
		Attributes:  attributes,
		Created:     created,
	}

	return r, nil
}

type SetupDuckDBDatabaseOptions struct {
	Dimensions   int
	DatabasePath string
}

func SetupDuckDBDatabase(ctx context.Context, db *sql.DB, opts *SetupDuckDBDatabaseOptions) error {

	t1 := time.Now()

	defer func() {
		slog.Debug("Finished setting up database", "time", time.Since(t1))
	}()

	cmds := make([]string, 0)

	q := "SELECT CAST(1 AS BOOL) AS vss FROM duckdb_extensions() WHERE installed = true AND loaded = true AND extension_name = 'vss'"

	row := db.QueryRowContext(ctx, q)

	var has_vss bool
	err := row.Scan(&has_vss)

	if err != nil {

		if err != sql.ErrNoRows {
			return fmt.Errorf("Failed to determine whether VSS extension is loaded, %w", err)
		}

		has_vss = false
	}

	if has_vss {
		slog.Debug("Statically linked VSS extension installed and loaded")
	} else {
		cmds = append(cmds, "INSTALL VSS")
		cmds = append(cmds, "LOAD VSS")
	}

	import_db := false

	if opts.DatabasePath != "" {

		import_db = true

		ensure_present := []string{
			"embeddings.csv",
			"load.sql",
			"schema.sql",
		}

		for _, path := range ensure_present {

			path = filepath.Join(opts.DatabasePath, path)

			info, err := os.Stat(path)

			if err != nil {
				slog.Debug("Required database path not present", "path", path, "error", err)
				import_db = false
				break
			}

			if info.IsDir() {
				slog.Debug("Required database is a directory", "path", path)
				import_db = false
				break
			}
		}

	}

	if import_db {
		slog.Debug("Load database from path", "path", opts.DatabasePath)
		cmds = append(cmds, fmt.Sprintf("IMPORT DATABASE '%s'", opts.DatabasePath))
	} else {
		cmds = append(cmds, fmt.Sprintf("CREATE TABLE embeddings(provider TEXT NOT NULL, depiction_id TEXT NOT NULL, subject_id TEXT NOT NULL, model TEXT NOT NULL, attributes TEXT NOT NULL, vec FLOAT[%d], created BIGINT NOT NULL, lastmodified BIGINT NOT NULL)", opts.Dimensions))
		cmds = append(cmds, "CREATE UNIQUE INDEX id_model ON embeddings (provider, depiction_id, model)")
		cmds = append(cmds, "CREATE INDEX by_provider ON embeddings (provider, model, created)")
		cmds = append(cmds, "CREATE INDEX by_model ON embeddings (model, provider, created)")
		cmds = append(cmds, "CREATE INDEX by_lastmod ON embeddings (lastmodified)")
		cmds = append(cmds, "CREATE INDEX idx ON embeddings USING HNSW (vec)")
	}

	for _, q := range cmds {

		slog.Debug(q)

		_, err := db.ExecContext(ctx, q)

		if err != nil {
			return fmt.Errorf("Failed to configure data - query failed, %w (%s)", err, q)
		}
	}

	return nil
}
