package parquet

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/sfomuseum/go-embeddingsdb/client"
	"github.com/sfomuseum/go-embeddingsdb/database"
)

// ImportRemote [*embeddingsdb.Record] records stored in a remote Parquet file identified by 'uri' and add them to an embeddings database using 'cl'.
func ImportRemote(ctx context.Context, cl client.Client, uri *url.URL) (int64, error) {

	logger := slog.Default()
	count := int64(0)

	db, err := sql.Open("duckdb", "")

	if err != nil {
		return count, err
	}

	defer db.Close()

	ticker := time.NewTicker(60 * time.Second)
	done_ch := make(chan bool)

	defer func() {
		ticker.Stop()
		done_ch <- true
	}()

	go func() {

		for {
			select {
			case <-done_ch:
				logger.Debug("Records imported", "count", count)
				return
			case <-ticker.C:
				logger.Debug("Records imported", "count", count)
			}
		}
	}()

	q := fmt.Sprintf(`SELECT provider, depiction_id, subject_id, model, embeddings, created, CAST(TO_JSON(attributes) AS VARCHAR) AS attributes FROM read_parquet('%s')`, uri.String())

	rows, err := db.QueryContext(ctx, q)

	if err != nil {
		return count, err
	}

	defer rows.Close()

	for rows.Next() {

		row, err := database.InflateDuckDBRecord(ctx, rows)

		if err != nil {
			logger.Error("Failed to inflate row", "error", err)
			return count, err
		}

		err = cl.AddRecord(ctx, row)

		if err != nil {
			return count, fmt.Errorf("Failed to add record '%s', %w", row.Key(), err)
		}

		count += 1
		logger.Debug("Add record", "key", row.Key(), "total", count)
	}

	err = rows.Close()

	if err != nil {
		return count, err
	}

	err = rows.Err()

	if err != nil {
		return count, err
	}

	return count, nil
}
