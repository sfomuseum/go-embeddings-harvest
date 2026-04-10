package client

import (
	"context"
	"fmt"
	"iter"
	"log/slog"

	"github.com/aaronland/go-pagination/countable"
	"github.com/sfomuseum/go-embeddingsdb"
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
func ListRecords(ctx context.Context, cl Client, opts *ListRecordsOptions) iter.Seq2[*embeddingsdb.Record, error] {

	return func(yield func(*embeddingsdb.Record, error) bool) {

		current_page := opts.StartPage
		pages := int64(0)

		pg_opts, err := countable.NewCountableOptions()

		if err != nil {
			yield(nil, fmt.Errorf("Failed to create pagination options, %w", err))
			return
		}

		pg_opts.PerPage(opts.PerPage)

		logger := slog.Default()
		logger = logger.With("start page", opts.StartPage)
		logger = logger.With("end page", opts.EndPage)
		logger = logger.With("per page", opts.PerPage)

		logger.Debug("Start pagination")

		for pages == 0 || current_page <= pages {

			pg_opts.Pointer(current_page)

			logger.Debug("Query records", "page", current_page, "total page count", pages)
			records, pg_rsp, err := cl.ListRecords(ctx, pg_opts)

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

			if opts.EndPage != -1 && current_page >= opts.EndPage {
				logger.Debug("End page reached", "page", current_page)
				break
			}

			current_page += 1
		}
	}
}
