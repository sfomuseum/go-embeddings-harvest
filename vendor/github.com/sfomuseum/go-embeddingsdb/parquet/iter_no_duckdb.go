//go:build no_duckdb

package parquet

import (
	"context"
	"fmt"
	"iter"
	"log/slog"

	"github.com/sfomuseum/go-embeddingsdb"
)

func Iterate(ctx context.Context, uri string) iter.Seq2[*embeddingsdb.Record, error] {

	logger := slog.Default()
	logger = logger.With("uri", uri)

	return func(yield func(*embeddingsdb.Record, error) bool) {

		yield(nil, fmt.Errorf("This requires being compiled with the duckdb tag enabled"))
		return
	}
}
