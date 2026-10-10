package database

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"

	"github.com/aaronland/go-pagination"
	"github.com/jtacoma/uritemplates"
)

// MultiDatabaseDirection enumerates the direction of pagination for a MultiDatabase cursor.
type MultiDatabaseDirection string

const (
	// DirectionNext indicates that the pagination cursor points to the next page.
	DirectionNext MultiDatabaseDirection = "next"
	// DirectionPrevious indicates that the pagination cursor points to the previous page.
	DirectionPrevious MultiDatabaseDirection = "prev"
)

// MultiDatabaseCursorState represents the state required to paginate across multiple
// underlying databases.  It encodes the current database index, page number,
// cursor string (for cursor based pagination) and direction.
type MultiDatabaseCursorState struct {
	DatabaseIndex int                    `json:"db_idx"`
	Page          int64                  `json:"page"`
	Cursor        string                 `json:"cursor"`
	Direction     MultiDatabaseDirection `json:"dir"` // Keeps track of which way we are shifting
}

// String serializes the MultiDatabaseCursorState into a URL-safe Base64 encoded string.
func (c MultiDatabaseCursorState) String() string {

	b, err := json.Marshal(c)

	if err != nil {
		slog.Error("Failed to marshal cursor", "error", err)
		return ""
	}

	return base64.StdEncoding.EncodeToString(b)
}

// ParseCursorState decodes a Base64 string back into a MultiDatabaseCursorState.
func ParseCursorState(s string) (MultiDatabaseCursorState, error) {

	var state MultiDatabaseCursorState
	b, err := base64.StdEncoding.DecodeString(s)

	if err != nil {
		slog.Error("Failed to decode cursor state", "encoded", s, "error", err)
		return state, err
	}

	err = json.Unmarshal(b, &state)
	return state, err
}

// MultiDatabasePaginationResults implements pagination.Results for a MultiDatabase.
// It contains the total record count, per-page setting and next/previous cursor
// states for multi‑database pagination.
type MultiDatabasePaginationResults struct {
	pagination.Results
	perPage  int64
	total    int64
	next     *MultiDatabaseCursorState
	previous *MultiDatabaseCursorState
	method   pagination.Method
}

// Total returns the overall number of records across all databases.
func (m *MultiDatabasePaginationResults) Total() int64 {
	return m.total
}

// PerPage returns the number of records requested per page.
func (m *MultiDatabasePaginationResults) PerPage() int64 {
	return m.perPage
}

// Page returns the current page number; for multi‑database pagination this is
// always 0 because the concept of a single page number does not apply.
func (m *MultiDatabasePaginationResults) Page() int64 {
	return 0
}

// Pages returns the total number of pages given the per-page value and total
// records.  It handles a zero perPage by returning 0.
func (m *MultiDatabasePaginationResults) Pages() int64 {

	if m.perPage == 0 {
		return 0
	}

	return (m.total + m.perPage - 1) / m.perPage
}

// Next returns the cursor that points to the next page.  The returned value
// is of type *MultiDatabaseCursorState.
func (m *MultiDatabasePaginationResults) Next() any {
	return m.next
}

// Previous returns the cursor that points to the previous page.
func (m *MultiDatabasePaginationResults) Previous() any {
	return m.previous
}

// Method reports the pagination method used (always cursor for MultiDatabase).
func (m *MultiDatabasePaginationResults) Method() pagination.Method {
	return m.method
}

// NextURL expands the provided UriTemplate with the next cursor and per‑page
// parameters to generate the URL for the next page.  If no next cursor is present,
// an empty string is returned.
func (m *MultiDatabasePaginationResults) NextURL(t *uritemplates.UriTemplate) (string, error) {

	if m.next == nil || t == nil {
		return "", nil
	}

	return t.Expand(map[string]interface{}{
		"cursor":   m.next.String(),
		"per_page": m.perPage,
	})
}

// PreviousURL expands the provided UriTemplate with the previous cursor and
// per‑page parameters to generate the URL for the previous page.  If no
// previous cursor is present, an empty string is returned.
func (m *MultiDatabasePaginationResults) PreviousURL(t *uritemplates.UriTemplate) (string, error) {

	if m.previous == nil || t == nil {
		return "", nil
	}

	return t.Expand(map[string]interface{}{
		"cursor":   m.previous.String(),
		"per_page": m.perPage,
	})
}
