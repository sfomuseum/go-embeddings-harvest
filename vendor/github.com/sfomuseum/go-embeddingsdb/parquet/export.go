package parquet

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	parquet_go "github.com/parquet-go/parquet-go"
	"github.com/sfomuseum/go-embeddingsdb"
	"github.com/sfomuseum/go-embeddingsdb/client"
)

// Export will export all the [*embeddingsdb.Record] records stored in 'db' as Parquet-encoded data to 'wr'.
func Export(ctx context.Context, cl client.Client, wr io.Writer) (int64, error) {

	logger := slog.Default()
	p_wr := parquet_go.NewGenericWriter[*embeddingsdb.Record](wr)

	ticker := time.NewTicker(60 * time.Second)
	done_ch := make(chan bool)

	count := int64(0)

	defer func() {
		ticker.Stop()
		done_ch <- true
	}()

	go func() {

		for {
			select {
			case <-done_ch:
				logger.Debug("Records exported", "count", count)
				return
			case <-ticker.C:
				logger.Debug("Records exported", "count", count)
			}
		}
	}()

	list_opts := client.DefaultListRecordsOptions()

	for row, err := range client.ListRecords(ctx, cl, list_opts) {

		if err != nil {
			return count, fmt.Errorf("Iterator yielded an error, %w", err)
		}

		_, err = p_wr.Write([]*embeddingsdb.Record{
			row,
		})

		if err != nil {
			return count, fmt.Errorf("Failed to export %s, %w", row.Key(), err)
		}

		count += 1
	}

	p_wr.Flush()

	err := p_wr.Close()

	if err != nil {
		return count, fmt.Errorf("Failed to close Parquet writer, %w", err)
	}

	return count, nil
}
