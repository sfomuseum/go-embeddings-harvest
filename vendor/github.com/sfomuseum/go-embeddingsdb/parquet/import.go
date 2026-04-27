package parquet

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sfomuseum/go-embeddingsdb/client"
)

// Import [*embeddingsdb.Record] records stored in one or more Parquet files identified by 'uris' and add them to an embeddings database using 'cl'.
func Import(ctx context.Context, cl client.Client, uris ...string) (int64, error) {

	logger := slog.Default()
	total := int64(0)

	for _, uri := range uris {

		logger := slog.Default()
		logger = logger.With("uri", uri)

		count := int64(0)

		for rec, err := range Iterate(ctx, uri) {

			if err != nil {
				logger.Error("Iterator yielded an error", "error", err)
				return total, err
			}

			err := cl.AddRecord(ctx, rec)

			if err != nil {
				logger.Error("Failed to add record", "key", rec.Key(), "error", err)
				return total, fmt.Errorf("Failed to add record '%s', %w", rec.Key(), err)
			}

			count += 1
			total += 1

			logger.Debug("Add record", "key", rec.Key(), "count", count, "total", total)
		}

		logger.Debug("Finished iterating uri", "count", count, "total", total)
	}

	logger.Debug("Finished importing all", "total", total)
	return total, nil
}
