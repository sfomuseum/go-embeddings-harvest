//go:build !no_duckdb

package parquet

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	parquet_go "github.com/parquet-go/parquet-go"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/database"
)

// To do: Add filtering mechanism
// To do: Support reading from remote storage using gocloud.dev/blob

func Iterate(ctx context.Context, uris ...string) iter.Seq2[*embeddingsdb.Record, error] {

	return func(yield func(*embeddingsdb.Record, error) bool) {

		for _, uri := range uris {

			logger := slog.Default()
			logger = logger.With("uri", uri)

			switch {
			case strings.HasPrefix(uri, "http"):

				_, err := url.Parse(uri)

				if err != nil {
					logger.Error("Failed to parse URI", "error", err)
					yield(nil, fmt.Errorf("Failed to parse URI, %w", err))
					return
				}

				db, err := sql.Open("duckdb", "")

				if err != nil {
					logger.Error("Failed to open DuckDB", "error", err)
					yield(nil, fmt.Errorf("Failed to open DuckDB, %w", err))
					return
				}

				defer db.Close()

				q := fmt.Sprintf(`SELECT provider, depiction_id, subject_id, model, embeddings, created, CAST(TO_JSON(attributes) AS VARCHAR) AS attributes FROM read_parquet('%s')`, uri)

				rows, err := db.QueryContext(ctx, q)

				if err != nil {
					logger.Error("Failed to query Parquet file", "error", err)
					yield(nil, fmt.Errorf("Failed to query Parquet file, %w", err))
					return
				}

				defer rows.Close()

				for rows.Next() {

					select {
					case <-ctx.Done():
						logger.Debug("Context signaled done, exiting")
						return
					default:

						rec, err := database.InflateDuckDBRecord(ctx, rows)

						if err != nil {
							logger.Error("Failed to inflate record", "error", err)
						}

						if !yield(rec, err) {
							return
						}
					}
				}

				err = rows.Close()

				if err != nil {
					logger.Error("Failed to close database rows", "error", err)
					yield(nil, fmt.Errorf("Failed to close database rows, %w", err))
					return
				}

				err = rows.Err()

				if err != nil {
					logger.Error("Database returned an error", "error", err)
					yield(nil, fmt.Errorf("Database returned an error, %w", err))
				}

			default:

				batch_size := 100

				abs_path, err := filepath.Abs(uri)

				if err != nil {
					logger.Error("Failed to derive absolute path for URI", "error", err)
					yield(nil, fmt.Errorf("Failed to derive absolute path for URI, %w", err))
					return
				}

				r, err := os.Open(abs_path)

				if err != nil {
					logger.Error("Failed to open URI", "path", abs_path, "error", err)
					yield(nil, fmt.Errorf("Failed to open URI for reading, %w", err))
					return
				}

				defer r.Close()

				parquet_r := parquet_go.NewGenericReader[*embeddingsdb.Record](r)
				buf := make([]*embeddingsdb.Record, batch_size)

				for {

					select {
					case <-ctx.Done():
						logger.Debug("Context signaled done, exiting")
						return
					default:

						n, err := parquet_r.Read(buf)

						switch {
						case err == io.EOF:
							return
						case err != nil:
							logger.Error("Parquet reader failed", "error", err)
							yield(nil, fmt.Errorf("Parquet reader failed, %w", err))
							return
						}

						for _, rec := range buf[:n] {
							if !yield(rec, nil) {
								return
							}
						}
					}
				}

			}
		}
	}
}
