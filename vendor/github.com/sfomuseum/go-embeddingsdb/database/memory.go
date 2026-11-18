package database

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"math"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/aaronland/go-pagination"
	"github.com/aaronland/go-pagination/countable"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/options"
)

const MemoryDatabaseScheme string = "memory"

// MemoryDatabase is an implementation of the [Database] interface that stores records in memory.
// Similar records are found using a brute-force (exact) Euclidean (L2) distance search.
type MemoryDatabase struct {
	db_uri       string
	dimensions   int
	max_distance float32
	max_results  int32
	mu           sync.RWMutex
	records      map[string]*embeddingsdb.Record
	lastupdate   int64
}

func init() {

	ctx := context.Background()
	err := RegisterDatabase(ctx, MemoryDatabaseScheme, NewMemoryDatabase)

	if err != nil {
		panic(err)
	}
}

// NewMemoryDatabase returns an in-memory implementation of the [Database] interface configured by 'uri'
// which is expected to take the form of:
//
//	memory://?{QUERY_PARAMETERS}
//
// Where {QUERY_PARAMETERS) may be:
// * `dimensions` - The number of dimensions for embeddings. If not set (or 0) the dimensions of records are not validated.
// * `max-distance` - The maximum distance between records when performing a similar records query. Default is 1.0.
// * `max-results` - The maximum number of results when performing a similar records query. Default is 10.
func NewMemoryDatabase(ctx context.Context, uri string) (Database, error) {

	u, err := url.Parse(uri)

	if err != nil {
		return nil, fmt.Errorf("Failed to parse URI, %w", err)
	}

	q := u.Query()

	dimensions := 0
	max_distance := float32(1.0)
	max_results := int32(10)

	if q.Has("dimensions") {

		v, err := strconv.Atoi(q.Get("dimensions"))

		if err != nil || v < 0 {
			return nil, fmt.Errorf("Invalid ?dimensions= parameter")
		}

		dimensions = v
	}

	if q.Has("max-distance") {

		v, err := strconv.ParseFloat(q.Get("max-distance"), 32)

		if err != nil {
			return nil, fmt.Errorf("Invalid ?max-distance= parameter, %w", err)
		}

		max_distance = float32(v)
	}

	if q.Has("max-results") {

		v, err := strconv.Atoi(q.Get("max-results"))

		if err != nil || v < 1 {
			return nil, fmt.Errorf("Invalid ?max-results= parameter")
		}

		max_results = int32(v)
	}

	db := &MemoryDatabase{
		db_uri:       uri,
		dimensions:   dimensions,
		max_distance: max_distance,
		max_results:  max_results,
		records:      make(map[string]*embeddingsdb.Record),
	}

	return db, nil
}

// Return the URI string used to instantiate the Database instance.
func (db *MemoryDatabase) URI() string {
	return db.db_uri
}

// AddRecord adds (or replaces) a [embeddingsdb.Record] instance. Records are never batched.
func (db *MemoryDatabase) AddRecord(ctx context.Context, rec *embeddingsdb.Record, opts ...options.Option) (bool, error) {

	if rec == nil {
		return false, fmt.Errorf("Missing record")
	}

	if db.dimensions > 0 && len(rec.Embeddings) != db.dimensions {
		return false, fmt.Errorf("Invalid embeddings dimensions for %s, expected %d but got %d", rec.Key(), db.dimensions, len(rec.Embeddings))
	}

	c := cloneRecord(rec)

	db.mu.Lock()
	defer db.mu.Unlock()

	db.records[c.Key()] = c
	db.lastupdate = time.Now().Unix()

	return false, nil
}

// The number of batched records currently waiting to be added (always 0).
func (db *MemoryDatabase) BatchedRecordsCount(ctx context.Context, opts ...options.Option) (int, error) {
	return 0, nil
}

// Add the pending batched records (a no-op).
func (db *MemoryDatabase) AddBatchedRecords(ctx context.Context, opts ...options.Option) error {
	return nil
}

// Return the EmbeddingsDB instance record matching 'provider', 'depiction_id' and 'model'.
func (db *MemoryDatabase) GetRecord(ctx context.Context, req *embeddingsdb.GetRecordRequest, opts ...options.Option) (*embeddingsdb.Record, error) {

	db.mu.RLock()
	defer db.mu.RUnlock()

	r, exists := db.records[req.Key()]

	if !exists {
		return nil, RecordNotFound
	}

	return cloneRecord(r), nil
}

// Remove a record from an EmbeddingsDB instance.
func (db *MemoryDatabase) RemoveRecord(ctx context.Context, req *embeddingsdb.RemoveRecordRequest, opts ...options.Option) error {

	db.mu.Lock()
	defer db.mu.Unlock()

	key := req.Key()

	if _, exists := db.records[key]; !exists {
		return RecordNotFound
	}

	delete(db.records, key)
	db.lastupdate = time.Now().Unix()

	return nil
}

// ListRecords returns a paginated (countable) list of records stored in the database, ordered by subject ID,
// depiction ID and model. Records may be filtered using [options.FilterOption] options.
func (db *MemoryDatabase) ListRecords(ctx context.Context, pg_opts pagination.Options, opts ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error) {

	filters := options.GetAllFiltersFromOptions(ctx, opts...)

	matches := make([]*embeddingsdb.Record, 0)

	db.mu.RLock()

	for _, r := range db.records {

		if matchesFilters(r, filters) {
			matches = append(matches, r)
		}
	}

	db.mu.RUnlock()

	sortRecords(matches)

	total := int64(len(matches))

	per_page := pg_opts.PerPage()
	page := countable.PageFromOptions(pg_opts)

	if page < 1 {
		page = 1
	}

	start := (page - 1) * per_page
	end := start + per_page

	if start > total {
		start = total
	}

	if end > total {
		end = total
	}

	records := make([]*embeddingsdb.Record, 0, end-start)

	for _, r := range matches[start:end] {
		records = append(records, cloneRecord(r))
	}

	pg, err := countable.NewResultsFromCountWithOptions(pg_opts, total)

	if err != nil {
		return nil, nil, fmt.Errorf("Failed to derive pagination results, %w", err)
	}

	return records, pg, nil
}

// IterateRecords returns an [iter.Seq2[*embeddingsdb.Record, error]] for each record stored in the database.
// Iteration is over a snapshot of the records taken when iteration begins.
func (db *MemoryDatabase) IterateRecords(ctx context.Context, opts ...options.Option) iter.Seq2[*embeddingsdb.Record, error] {

	return func(yield func(*embeddingsdb.Record, error) bool) {

		db.mu.RLock()
		snapshot := slices.Collect(maps.Values(db.records))
		db.mu.RUnlock()

		sortRecords(snapshot)

		for _, r := range snapshot {

			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}

			if !yield(cloneRecord(r), nil) {
				return
			}
		}
	}
}

// CountRecords returns the total number of records in the database.
func (db *MemoryDatabase) CountRecords(ctx context.Context, opts ...options.Option) (int64, error) {

	db.mu.RLock()
	defer db.mu.RUnlock()

	return int64(len(db.records)), nil
}

// SimilarRecords finds the records for 'req.Model' whose embeddings are nearest (by Euclidean distance) to 'req.Embeddings',
// closest first. Records with a distance of 0, beyond the maximum distance, or with a depiction ID in 'req.Exclude'
// are omitted. Results may be tuned using the max distance, max results and similar provider options.
func (db *MemoryDatabase) SimilarRecords(ctx context.Context, req *embeddingsdb.SimilarRecordsRequest, opts ...options.Option) ([]*embeddingsdb.SimilarRecord, error) {

	max_distance := db.max_distance
	max_results := int(db.max_results)

	if v := options.GetMaxDistanceFromOptions(ctx, opts...); v != nil {
		max_distance = *v
	}

	if v := options.GetMaxResultsFromOptions(ctx, opts...); v != nil {
		max_results = int(*v)
	}

	similar_provider := options.GetSimilarProviderFromOptions(ctx, opts...)

	if db.dimensions > 0 && len(req.Embeddings) != db.dimensions {
		return nil, fmt.Errorf("Invalid embeddings dimensions, expected %d but got %d", db.dimensions, len(req.Embeddings))
	}

	exclude := make(map[string]bool, len(req.Exclude))

	for _, id := range req.Exclude {
		exclude[id] = true
	}

	results := make([]*embeddingsdb.SimilarRecord, 0)

	db.mu.RLock()

	for _, r := range db.records {

		if r.Model != req.Model {
			continue
		}

		if similar_provider != nil && r.Provider != *similar_provider {
			continue
		}

		if exclude[r.DepictionId] {
			continue
		}

		if len(r.Embeddings) != len(req.Embeddings) {
			continue
		}

		d := l2Distance(req.Embeddings, r.Embeddings)

		if d <= 0 || d > max_distance {
			continue
		}

		attrs := make(map[string]string, len(r.Attributes))
		maps.Copy(attrs, r.Attributes)

		results = append(results, &embeddingsdb.SimilarRecord{
			Provider:    r.Provider,
			DepictionId: r.DepictionId,
			SubjectId:   r.SubjectId,
			Distance:    d,
			Attributes:  attrs,
		})
	}

	db.mu.RUnlock()

	sort.SliceStable(results, func(i, j int) bool {

		if results[i].Distance != results[j].Distance {
			return results[i].Distance < results[j].Distance
		}

		return results[i].DepictionId < results[j].DepictionId
	})

	if max_results > 0 && len(results) > max_results {
		results = results[:max_results]
	}

	return results, nil
}

// Export is not supported by the in-memory database.
func (db *MemoryDatabase) Export(ctx context.Context, uri string, opts ...options.Option) error {
	return NotImplemented
}

// Return the Unix timestamp of the last update to the Database instance.
func (db *MemoryDatabase) LastUpdate(ctx context.Context, opts ...options.Option) (int64, error) {

	db.mu.RLock()
	defer db.mu.RUnlock()

	return db.lastupdate, nil
}

// Return the list of dimensions supported by this Database implementation. If no dimensions were
// configured then the distinct dimensions of the stored embeddings are returned.
func (db *MemoryDatabase) Dimensions(ctx context.Context, opts ...options.Option) ([]int, error) {

	if db.dimensions > 0 {
		return []int{db.dimensions}, nil
	}

	seen := make(map[int]bool)

	db.mu.RLock()

	for _, r := range db.records {
		seen[len(r.Embeddings)] = true
	}

	db.mu.RUnlock()

	dimensions := slices.Collect(maps.Keys(seen))
	slices.Sort(dimensions)

	return dimensions, nil
}

// Return the unique list of models, for zero (all) or more providers, across all the embeddings.
func (db *MemoryDatabase) Models(ctx context.Context, opts ...options.Option) ([]string, error) {

	providers := options.GetAllProvidersFromOptions(ctx, opts...)

	seen := make(map[string]bool)

	db.mu.RLock()

	for _, r := range db.records {

		if len(providers) > 0 && !slices.Contains(providers, r.Provider) {
			continue
		}

		seen[r.Model] = true
	}

	db.mu.RUnlock()

	models := slices.Collect(maps.Keys(seen))
	slices.Sort(models)

	return models, nil
}

// Return the unique list of providers across all the embeddings.
func (db *MemoryDatabase) Providers(ctx context.Context, opts ...options.Option) ([]string, error) {

	seen := make(map[string]bool)

	db.mu.RLock()

	for _, r := range db.records {
		seen[r.Provider] = true
	}

	db.mu.RUnlock()

	providers := slices.Collect(maps.Keys(seen))
	slices.Sort(providers)

	return providers, nil
}

// Return the pagination type used by the database.
func (db *MemoryDatabase) PaginationType(ctx context.Context, opts ...options.Option) (PaginationType, error) {
	return CountablePaginationType, nil
}

// Close discards all the records in the database.
func (db *MemoryDatabase) Close(ctx context.Context) error {

	db.mu.Lock()
	defer db.mu.Unlock()

	db.records = make(map[string]*embeddingsdb.Record)
	return nil
}

func cloneRecord(r *embeddingsdb.Record) *embeddingsdb.Record {

	c := *r
	c.Embeddings = slices.Clone(r.Embeddings)

	if r.Attributes != nil {
		c.Attributes = maps.Clone(r.Attributes)
	}

	return &c
}

func sortRecords(records []*embeddingsdb.Record) {

	sort.Slice(records, func(i, j int) bool {

		a, b := records[i], records[j]

		if a.SubjectId != b.SubjectId {
			return a.SubjectId < b.SubjectId
		}

		if a.DepictionId != b.DepictionId {
			return a.DepictionId < b.DepictionId
		}

		if a.Model != b.Model {
			return a.Model < b.Model
		}

		return a.Provider < b.Provider
	})
}

func matchesFilters(r *embeddingsdb.Record, filters []*options.FilterOption) bool {

	for _, f := range filters {

		var v string

		switch f.Key() {
		case "provider":
			v = r.Provider
		case "depiction_id":
			v = r.DepictionId
		case "subject_id":
			v = r.SubjectId
		case "model":
			v = r.Model
		default:
			return false
		}

		if v != fmt.Sprintf("%v", f.Value()) {
			return false
		}
	}

	return true
}

func l2Distance(a, b []float32) float32 {

	var sum float64

	for i := range a {
		d := float64(a[i] - b[i])
		sum += d * d
	}

	return float32(math.Sqrt(sum))
}
