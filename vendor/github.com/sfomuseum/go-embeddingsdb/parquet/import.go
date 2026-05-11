package parquet

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sfomuseum/go-embeddingsdb/client"
)

// Import [*embeddingsdb.Record] records stored in one or more Parquet files identified by 'uris' and add them to an embeddings database using 'cl'.
func Import(ctx context.Context, cl client.Client, uris ...string) (int64, error) {

	start := int64(0)
	end := int64(0)

	return ImportWithRange(ctx, cl, start, end, uris...)
}

// ImportWithRange [*embeddingsdb.Record] records whose position is between 'start' and 'end' (inclusive) stored in one or more Parquet files identified by 'uris' and add them to an embeddings database using 'cl'.
func ImportWithRange(ctx context.Context, cl client.Client, start int64, end int64, uris ...string) (int64, error) {

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

			count += 1
			total += 1

			if start > 0 && start > count {
				continue
			}

			err := cl.AddRecord(ctx, rec)

			if err != nil {
				logger.Error("Failed to add record", "key", rec.Key(), "error", err)
				return total, fmt.Errorf("Failed to add record '%s', %w", rec.Key(), err)
			}

			logger.Debug("Add record", "key", rec.Key(), "count", count, "total", total)

			if end > 0 && end >= count {
				break
			}
		}

		logger.Debug("Finished iterating uri", "count", count, "total", total)
	}

	logger.Debug("Finished importing all", "total", total)
	return total, nil
}
