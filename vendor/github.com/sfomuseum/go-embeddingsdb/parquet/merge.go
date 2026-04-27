package parquet

import (
	"context"
	"log/slog"

	"github.com/sfomuseum/go-embeddingsdb"
)

// To do: Add filtering mechanism

func Merge(ctx context.Context, wr *ParquetWriter, uris ...string) (int64, error) {

	defer wr.Flush()

	logger := slog.Default()

	rows := make([]*embeddingsdb.Record, 0)
	batch_size := 500
	total := int64(0)

	for _, uri := range uris {

		count := int64(0)

		for row, err := range Iterate(ctx, uri) {

			if err != nil {
				logger.Error("Iterator yielded an error", "uri", uri, "error", err)
				return total, err
			}

			rows = append(rows, row)

			if len(rows) >= batch_size {

				_, err := wr.Write(rows)

				if err != nil {
					logger.Error("Failed to write rows", "uri", uri, "error", err)
					return total, err
				}

				rows = make([]*embeddingsdb.Record, 0)
			}

			count += 1
			total += 1
		}

		logger.Debug("Finished iterating uri", "uri", uri, "count", count, "total", total, "pending", len(rows))
	}

	logger.Debug("Remaining rows", "count", len(rows))

	if len(rows) > 0 {

		_, err := wr.Write(rows)

		if err != nil {
			logger.Error("Failed to write rows", "error", err)
			return total, err
		}

		logger.Debug("Wrote pending rows", "count", len(rows))
		wr.Flush()

		total += int64(len(rows))
	}

	logger.Debug("Finished iterating all", "total", total)

	return total, nil
}
