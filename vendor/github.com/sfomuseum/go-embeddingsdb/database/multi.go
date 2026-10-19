package database

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/aaronland/go-pagination"
	"github.com/aaronland/go-pagination/countable"
	"github.com/aaronland/go-pagination/cursor"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/options"
)

// MultiDatabaseScheme is the URI scheme used for a database that aggregates multiple
// underlying databases.
const MultiDatabaseScheme string = "multi"

// MultiDatabase represents a database that contains one or more underlying databases,
// each of which may support a different number of embedding dimensions or a different
// pagination strategy. It implements the Database interface.
type MultiDatabase struct {
	Database
	databases       []Database
	lookup          map[int]int // map dimensions to Database offset in `databases`
	pg_lookup       map[int]PaginationType
	model_cache     *sync.Map
	pagination_type PaginationType
}

func init() {

	ctx := context.Background()
	err := RegisterDatabase(ctx, MultiDatabaseScheme, NewMultiDatabase)

	if err != nil {
		panic(err)
	}
}

// NewMultiDatabaseURIFromURIs creates a URI string for a MultiDatabase from a list of
// underlying database URIs. The returned string uses the "multi" scheme and
// encodes each supplied URI as a query parameter named "database".
func NewMultiDatabaseURIFromURIs(db_uris ...string) string {

	db_q := url.Values{}
	db_q["database"] = db_uris

	db_u := url.URL{}
	db_u.Scheme = "multi"
	db_u.RawQuery = db_q.Encode()

	return db_u.String()
}

// NewMultiDatabase constructs a MultiDatabase instance from a URI. The URI must
// use the "multi" scheme and contain one or more "?database=" query parameters
// that point to the underlying databases. It returns an error if parsing fails
// or if the URI is missing the required parameters.
func NewMultiDatabase(ctx context.Context, uri string) (Database, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, err
	}

	q := u.Query()

	db_uris, ok := q["database"]

	if !ok {
		return nil, fmt.Errorf("URI missing one or more ?database= parameters")
	}

	return NewMultiDatabaseFromURIs(ctx, db_uris...)
}

// NewMultiDatabaseFromURIs constructs a MultiDatabase from a list of database
// URIs. Each URI must contain a "dimensions" query parameter that specifies
// the dimensionality of the embeddings it contains. The function ensures that
// no two databases share the same dimensionality.
func NewMultiDatabaseFromURIs(ctx context.Context, db_uris ...string) (Database, error) {

	registry := make(map[int]Database)

	for _, uri := range db_uris {

		db_u, err := url.Parse(uri)

		if err != nil {
			return nil, fmt.Errorf("Failed to parse database URI '%s', %w", uri, err)
		}

		db_q := db_u.Query()

		if !db_q.Has("dimensions") {
			return nil, fmt.Errorf("Database URI '%s' missing dimensions", uri)
		}

		str_dims := db_q.Get("dimensions")

		if str_dims == "" {
			return nil, fmt.Errorf("Database URI '%s' has empty dimensions", uri)
		}

		dims, err := strconv.Atoi(str_dims)

		if err != nil {
			return nil, fmt.Errorf("Failed to parse dimensions for database URI '%s', %w", uri, err)
		}

		_, exists := registry[dims]

		if exists {
			return nil, fmt.Errorf("Database already registered for dimensions '%d'", dims)
		}

		other_db, err := NewDatabase(ctx, uri)

		if err != nil {
			return nil, fmt.Errorf("Failed to create new database for '%s', %w", uri, err)
		}

		registry[dims] = other_db
	}

	return NewMultiDatabaseFromRegistry(ctx, registry)
}

// NewMultiDatabaseFromRegistry constructs a MultiDatabase from a map of
// dimensionality to Database instances. It determines the pagination type
// of each underlying database and prepares internal lookup tables.
func NewMultiDatabaseFromRegistry(ctx context.Context, registry map[int]Database) (Database, error) {

	databases := make([]Database, 0)
	lookup := make(map[int]int)
	pg_lookup := make(map[int]PaginationType)

	for dims, target_db := range registry {

		target_pg, err := target_db.PaginationType(ctx)

		if err != nil {
			return nil, err
		}

		databases = append(databases, target_db)
		idx := len(databases) - 1

		lookup[dims] = idx
		pg_lookup[idx] = target_pg

		slog.Debug("Add database", "index", idx, "dimensions", dims, "pagination type", target_pg)
	}

	model_cache := new(sync.Map)

	db := &MultiDatabase{
		databases:       databases,
		lookup:          lookup,
		pg_lookup:       pg_lookup,
		model_cache:     model_cache,
		pagination_type: MultiPaginationType,
	}

	return db, nil
}

// URI returns the URI string that was used to instantiate the MultiDatabase.
// The string contains the "multi" scheme and a list of the underlying database
// URIs as the "database" query parameter.
func (db *MultiDatabase) URI() string {

	database_uris := make([]string, 0)

	for _, target_db := range db.databases {

		target_uri := target_db.URI()
		database_uris = append(database_uris, target_uri)
	}

	q := url.Values{}
	q["database"] = database_uris

	u := url.URL{}
	u.Scheme = "multi"
	u.RawQuery = q.Encode()

	return u.String()
}

// Export the contents of the database. Where and how a database is exported are left as details for specific implementations.
func (db *MultiDatabase) Export(ctx context.Context, uri string, opts ...options.Option) error {

	for _, target_db := range db.databases {

		err := target_db.Export(ctx, uri, opts...)

		if err != nil {
			return err
		}
	}

	return nil
}

// Add adds a [embeddingsdb.Record] instance to the underlying database implementation. Returns true or false if the addition was batched.
func (db *MultiDatabase) AddRecord(ctx context.Context, rec *embeddingsdb.Record, opts ...options.Option) (bool, error) {

	dims := len(rec.Embeddings)

	target_db, err := db.databaseForDimensions(ctx, dims)

	if err != nil {
		return false, err
	}

	return target_db.AddRecord(ctx, rec, opts...)
}

// The number of batched records currently waiting to be added.
func (db *MultiDatabase) BatchedRecordsCount(ctx context.Context, opts ...options.Option) (int, error) {

	total := 0

	for _, target_db := range db.databases {

		count, err := target_db.BatchedRecordsCount(ctx, opts...)

		if err != nil {
			return total, err
		}

		total += count
	}

	return total, nil
}

// Add the pending batched records.
func (db *MultiDatabase) AddBatchedRecord(ctx context.Context, opts ...options.Option) error {

	for _, target_db := range db.databases {

		err := target_db.AddBatchedRecords(ctx, opts...)

		if err != nil {
			return err
		}
	}

	return nil
}

// Return the EmbeddingsDB instance record matching 'provider', 'depiction_id' and 'model'.
func (db *MultiDatabase) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest, opts ...options.Option) (*embeddingsdb.Record, error) {

	target_db, err := db.databaseForModel(ctx, req.Model, opts...)

	if err != nil {
		return nil, err

	}

	return target_db.GetRecord(ctx, req, opts...)
}

// Remove a record from an EmbeddingsDB instance.
func (db *MultiDatabase) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest, opts ...options.Option) error {

	target_db, err := db.databaseForModel(ctx, req.Model, opts...)

	if err != nil {
		return err
	}

	return target_db.RemoveRecord(ctx, req, opts...)
}

// Find similar records for a given model and record instance.
func (db *MultiDatabase) SimilarRecords(ctx context.Context, req *embeddingsdb.SimilarRecordsRequest, opts ...options.Option) ([]*embeddingsdb.SimilarRecord, error) {

	target_db, err := db.databaseForModel(ctx, req.Model, opts...)

	if err != nil {
		return nil, err
	}

	return target_db.SimilarRecords(ctx, req, opts...)
}

// ListRecords returns a paginated list of records stored in the database.  The
// method aggregates results from all underlying databases, honouring the
// pagination strategy of each.  The returned pagination results are of type
// MultiDatabasePaginationResults which encodes the next/previous state across
// multiple databases.
func (db *MultiDatabase) ListRecords(ctx context.Context, pg_opts pagination.Options, opts ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error) {

	var combined []*embeddingsdb.Record

	per_page := pg_opts.PerPage()
	remaining := per_page

	state := MultiDatabaseCursorState{
		DatabaseIndex: 0,
		Page:          1,
		Cursor:        "",
		Direction:     DirectionNext,
	}

	current_ptr := pg_opts.Pointer()

	if current_ptr != nil {

		switch current_ptr.(type) {
		case int64:
			// counter-based pagination, nothing to do
		case string:

			dec, err := ParseCursorState(current_ptr.(string))

			if err != nil {
				slog.Error("Failed to parse cursor string", "error", err, "cursor", current_ptr)
			} else {
				state = dec
			}

		case MultiDatabaseCursorState:
			state = current_ptr.(MultiDatabaseCursorState)
		default:
			slog.Warn("Unexpected type for pointer", "type", fmt.Sprintf("%T", current_ptr))
		}

	}

	// Capture the anchor point before we start mutating variables
	// This helps us build the "Previous" link accurately

	initial_state := state

	// Try to derive counts across all databases
	// Note the explicit timeout

	count_all := int64(0)
	count_ch := make(chan bool)

	go func() {

		count_ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		count, err := db.CountRecords(count_ctx, opts...)

		if err != nil {
			slog.Error("Failed to derive record(s) count", "error", err)
		}

		count_all = count
		count_ch <- true
	}()

	var last_pg pagination.Results

	// Query individual databases sequentially

	for i := state.DatabaseIndex; i < len(db.databases); i++ {

		target_db := db.databases[i]

		// Set up database-specific pagination options. This mostly
		// has to do with whether or not count-based pagination is
		// supported.

		var target_pg_opts pagination.Options

		target_pg_type := db.pg_lookup[i]

		logger := slog.Default()
		logger = logger.With("db index", i)
		logger = logger.With("pagination", target_pg_type)

		switch target_pg_type {
		case CountablePaginationType:

			countable_opts, err := countable.NewCountableOptions()

			if err != nil {
				return nil, nil, err
			}

			countable_opts.PerPage(remaining)
			countable_opts.Spill(0)
			countable_opts.Pointer(state.Page)
			target_pg_opts = countable_opts

		case CursorPaginationType:

			cursor_opts, err := cursor.NewCursorOptions()

			if err != nil {
				return nil, nil, err
			}

			cursor_opts.PerPage(remaining)
			cursor_opts.Pointer(state.Cursor)
			target_pg_opts = cursor_opts

		default:
			logger.Warn("Unsupported pagination type for database", "index", i)
			continue
		}

		logger.Debug("query database", "pointer", target_pg_opts.Pointer())

		records, pg, err := target_db.ListRecords(ctx, target_pg_opts, opts...)

		if err != nil {
			logger.Error("Failed to list records", "error", err)
			return nil, nil, fmt.Errorf("db cluster error at index %d: %w", i, err)
		}

		// Necessary below
		last_pg = pg

		combined = append(combined, records...)
		remaining -= int64(len(records))

		logger.Debug("Count", "remaining", remaining)

		if remaining <= 0 {

			next := &MultiDatabaseCursorState{
				DatabaseIndex: i,
				Direction:     DirectionNext,
			}

			switch target_pg_type {
			case CountablePaginationType:
				next.Page = pg.Page() + 1
			case CursorPaginationType:
				next.Cursor = pg.Next().(string)
			}

			// Calculate previous pointer based on where this request started
			var prev *MultiDatabaseCursorState

			if initial_state.DatabaseIndex > 0 || initial_state.Page > 1 {

				prev = &MultiDatabaseCursorState{
					DatabaseIndex: initial_state.DatabaseIndex,
					Direction:     DirectionPrevious,
				}

				switch target_pg_type {
				case CountablePaginationType:
					prev.Page = initial_state.Page - 1
				case CursorPaginationType:
					prev.Cursor = pg.Previous().(string)
				}

			}

			logger.Debug("Pagination", "prev", prev, "next", next)

			<-count_ch

			pg_rsp := &MultiDatabasePaginationResults{
				perPage:  per_page,
				total:    count_all,
				next:     next,
				previous: prev,
				method:   pagination.Cursor,
			}

			return combined, pg_rsp, nil
		}

		// Current database is empty/exhausted. Move forward, resetting the internal page tracking.
		state.Page = 1
	}

	// All databases crawled

	var prev *MultiDatabaseCursorState

	if initial_state.DatabaseIndex > 0 || initial_state.Page > 1 {

		prev = &MultiDatabaseCursorState{
			DatabaseIndex: initial_state.DatabaseIndex,
			Direction:     DirectionPrevious,
		}

		target_pg_type := db.pg_lookup[len(db.databases)-1]

		switch target_pg_type {
		case CountablePaginationType:
			prev.Page = initial_state.Page - 1
		case CursorPaginationType:

			if last_pg != nil {
				prev.Cursor = last_pg.Previous().(string)
			}
		}
	}

	<-count_ch

	pg_rsp := &MultiDatabasePaginationResults{
		perPage:  per_page,
		total:    count_all,
		next:     nil,
		previous: prev,
		method:   pagination.Cursor,
	}

	return combined, pg_rsp, nil
}

// CountRecords returns the total number of records indexed by all the registered databases.
// Depending on the database implementation this number may be approximate or not available.
func (db *MultiDatabase) CountRecords(ctx context.Context, opts ...options.Option) (int64, error) {

	count_all := int64(0)

	count_ch := make(chan int64)
	err_ch := make(chan error)
	done_ch := make(chan bool)

	db_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, target_db := range db.databases {

		go func(target_db Database) {

			defer func() {
				done_ch <- true
			}()

			count, err := target_db.CountRecords(db_ctx, opts...)

			if err != nil && err != NotAvailable {
				err_ch <- err
				return
			}

			count_ch <- count

		}(target_db)
	}

	remaining := len(db.databases)

	for remaining > 0 {
		select {
		case <-done_ch:
			remaining -= 1
		case err := <-err_ch:
			return count_all, err
		case count := <-count_ch:
			count_all += count
		}
	}

	return count_all, nil
}

// IterateRecords returns an [iter.Seq2[*embeddingsdb.Record, error]] for each record stored in the database.
func (db *MultiDatabase) IterateRecords(ctx context.Context, opts ...options.Option) iter.Seq2[*embeddingsdb.Record, error] {

	return func(yield func(*embeddingsdb.Record, error) bool) {

		keep_iterating := true

		for _, target_db := range db.databases {

			for rec, err := range target_db.IterateRecords(ctx, opts...) {

				if !yield(rec, err) {
					keep_iterating = false
					break
				}
			}

			if !keep_iterating {
				break
			}
		}
	}
}

// Return the Unix timestamp of the last update to the Database instance.
func (db *MultiDatabase) LastUpdate(ctx context.Context, opts ...options.Option) (int64, error) {

	lastupdate := int64(0)

	lastupdate_ch := make(chan int64)
	err_ch := make(chan error)
	done_ch := make(chan bool)

	db_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, target_db := range db.databases {

		go func(target_db Database) {

			defer func() {
				done_ch <- true
			}()

			u, err := target_db.LastUpdate(db_ctx, opts...)

			if err != nil {
				err_ch <- err
				return
			}

			lastupdate_ch <- u
		}(target_db)
	}

	remaining := len(db.databases)

	for remaining > 0 {
		select {
		case <-done_ch:
			remaining -= 1
		case err := <-err_ch:
			return lastupdate, err
		case t := <-lastupdate_ch:

			if t > lastupdate {
				lastupdate = t
			}
		}
	}

	return lastupdate, nil
}

// Return the list of dimensions supported by this Database  implementation.
func (db *MultiDatabase) Dimensions(ctx context.Context, opts ...options.Option) ([]int, error) {

	dims := make([]int, 0)

	for d, _ := range db.lookup {
		dims = append(dims, d)
	}

	return dims, nil
}

// Return the unique list of models, for zero (all) or more providers, across all the embeddings.
func (db *MultiDatabase) Models(ctx context.Context, opts ...options.Option) ([]string, error) {

	models := make([]string, 0)

	models_ch := make(chan []string)
	err_ch := make(chan error)
	done_ch := make(chan bool)

	db_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, target_db := range db.databases {

		go func(target_db Database) {

			defer func(){
				done_ch <- true
			}()
			
			target_models, err := target_db.Models(db_ctx, opts...)

			if err != nil {
				err_ch <- err
				return
			}

			models_ch <- target_models

		}(target_db)
	}

	remaining := len(db.databases)

	for remaining > 0 {
		select {
		case <-done_ch:
			remaining -= 1
		case err := <-err_ch:
			return models, err
		case candidates := <-models_ch:

			for _, m := range candidates {

				if !slices.Contains(models, m) {
					models = append(models, m)
				}
			}
		}
	}

	return models, nil
}

// Return the unique list of providers across all the embeddings.
func (db *MultiDatabase) Providers(ctx context.Context, opts ...options.Option) ([]string, error) {

	providers := make([]string, 0)

	providers_ch := make(chan []string)
	err_ch := make(chan error)
	done_ch := make(chan bool)

	db_ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, target_db := range db.databases {

		go func(target_db Database) {

			defer func() {
				done_ch <- true
			}()

			target_providers, err := target_db.Providers(db_ctx, opts...)

			if err != nil {
				err_ch <- err
				return
			}

			providers_ch <- target_providers

		}(target_db)
	}

	remaining := len(db.databases)

	for remaining > 0 {
		select {
		case <-done_ch:
			remaining -= 1
		case err := <-err_ch:
			return providers, err
		case candidates := <-providers_ch:

			for _, p := range candidates {

				if !slices.Contains(providers, p) {
					providers = append(providers, p)
				}
			}
		}
	}

	return providers, nil
}

// Return the pagination type used by the database.
func (db *MultiDatabase) PaginationType(ctx context.Context, opts ...options.Option) (PaginationType, error) {
	return MultiPaginationType, nil
}

// Close performs and terminating functions required by the database.
func (db *MultiDatabase) Close(ctx context.Context) error {

	for _, target_db := range db.databases {

		err := target_db.Close(ctx)

		if err != nil {
			return err
		}
	}

	return nil
}

func (db *MultiDatabase) databaseForDimensions(ctx context.Context, dims int) (Database, error) {

	target_idx, ok := db.lookup[dims]

	if !ok {
		return nil, fmt.Errorf("Unregistered database for %d dimensions", dims)
	}

	return db.databases[target_idx], nil
}

func (db *MultiDatabase) databaseForModel(ctx context.Context, model string, opts ...options.Option) (Database, error) {

	v, ok := db.model_cache.Load(model)

	if ok {

		switch v.(type) {
		case Database:
			return v.(Database), nil
		default:
			return nil, fmt.Errorf("Model not found")
		}
	}

	var target_db Database

	for _, test_db := range db.databases {

		test_models, err := test_db.Models(ctx, opts...)

		if err != nil {
			return nil, err
		}

		if slices.Contains(test_models, model) {
			target_db = test_db
			break
		}
	}

	db.model_cache.Store(model, target_db)

	if target_db == nil {
		return nil, fmt.Errorf("No matching database for model")
	}

	return target_db, nil
}
