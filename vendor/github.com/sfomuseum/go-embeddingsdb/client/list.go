package client

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"strings"

	"github.com/aaronland/go-pagination/countable"
	"github.com/aaronland/go-pagination/cursor"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/database"
	"github.com/sfomuseum/go-embeddingsdb/options"
)

// ListRecordOptions defines configuration options for calling the `ListRecords` method.
type ListRecordsOptions struct {
	// The number of records to return in each set of paginated results.
	PerPage int64
	// The initial page number to return paginated results for.
	StartPage int64
	// The maximum page number to return paginated results for. If -1 then this flag is ignored.
	EndPage int64
}

// DefaultListRecordsOptions returns a [ListRecordsOptions] with default values for
// returning all the records in an `embeddings` database in paginated sets of 1000
// records.
func DefaultListRecordsOptions() *ListRecordsOptions {

	opts := &ListRecordsOptions{
		PerPage:   int64(1000),
		StartPage: int64(1),
		EndPage:   int64(-1),
	}

	return opts
}

// ListRecords returns an [iter.Seq2[*embeddingsdb.Record, error]] iterator for listing all the records in
// an `embeddingsdb` database. It handles all the pagination requirements derived from 'opts'.
func ListRecords(ctx context.Context, cl Client, list_opts *ListRecordsOptions, opts ...options.Option) iter.Seq2[*embeddingsdb.Record, error] {

	return func(yield func(*embeddingsdb.Record, error) bool) {

		pg_type, err := cl.PaginationType(ctx)

		if err != nil {
			yield(nil, err)
			return
		}

		logger := slog.Default()

		switch pg_type {
		case database.CursorPaginationType:

			pg_opts, err := cursor.NewCursorOptions()

			if err != nil {
				yield(nil, fmt.Errorf("Failed to create pagination options, %w", err))
				return
			}

			pg_opts.PerPage(list_opts.PerPage)

			page := int64(1)

			for {

				logger.Debug("Query records", "pointer", pg_opts.Pointer())
				records, pg_rsp, err := cl.ListRecords(ctx, pg_opts, opts...)

				if err != nil {
					logger.Error("Failed to list records", "pointer", pg_opts.Pointer(), "error", err)
					yield(nil, fmt.Errorf("Failed to list records on with pointer %s, %w", pg_opts.Pointer(), err))
					return
				}

				if page < list_opts.StartPage {
					logger.Debug("Results before start page, skipping", "start", list_opts.StartPage, "page", page)
				} else {
					for _, r := range records {

						if !yield(r, nil) {
							logger.Warn("Iterator did not return true, exiting", "pointer", pg_opts.Pointer(), "record", r.Key())
							return
						}
					}
				}

				page += 1

				pg_next := pg_rsp.Next().(string)

				if pg_next == "" {
					logger.Debug("No more results")
					break
				}

				if list_opts.EndPage != -1 && page > list_opts.EndPage {
					logger.Debug("Arrived at requested end page", "end page", list_opts.EndPage, "page", page)
					break
				}

				pg_next = strings.Replace(pg_next, "after-", "", 1) // why did I add "after-" ...
				pg_opts.Pointer(pg_next)
				logger.Debug("Assign next cursor", "pointer", pg_next)
			}

		case database.CountablePaginationType:

			current_page := list_opts.StartPage
			pages := int64(0)

			pg_opts, err := countable.NewCountableOptions()

			if err != nil {
				yield(nil, fmt.Errorf("Failed to create pagination options, %w", err))
				return
			}

			pg_opts.PerPage(list_opts.PerPage)

			logger := slog.Default()
			logger = logger.With("start page", list_opts.StartPage)
			logger = logger.With("end page", list_opts.EndPage)
			logger = logger.With("per page", list_opts.PerPage)

			logger.Debug("Start pagination")

			for pages == 0 || current_page <= pages {

				pg_opts.Pointer(current_page)

				logger.Debug("Query records", "page", current_page, "total page count", pages)
				records, pg_rsp, err := cl.ListRecords(ctx, pg_opts, opts...)

				if err != nil {
					logger.Error("Failed to list records", "page", current_page, "error", err)
					yield(nil, fmt.Errorf("Failed to list records on page %d, %w", current_page, err))
					return
				}

				for _, r := range records {

					if !yield(r, nil) {
						logger.Warn("Iterator did not return true, exiting", "page", current_page, "record", r.Key())
						return
					}
				}

				if pages == 0 {
					logger.Debug("Assign total pages", "pages", pages)
					pages = pg_rsp.Pages()
				}

				if list_opts.EndPage != -1 && current_page >= list_opts.EndPage {
					logger.Debug("End page reached", "page", current_page)
					break
				}

				current_page += 1
			}
		default:
			yield(nil, fmt.Errorf("Unsupported pagination type %T", pg_type))
			return
		}
	}
}
