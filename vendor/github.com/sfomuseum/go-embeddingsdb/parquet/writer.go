package parquet

import (
	"context"
	"fmt"
	"io"
	"sync"

	parquet_go "github.com/parquet-go/parquet-go"
	"github.com/sfomuseum/go-embeddingsdb"
	sfom_parquet "github.com/sfomuseum/go-parquet"
)

// ParquetWriter is a convenience struct for wrapping the creation of both a Parquet "GenericWriter"
// and the underlying [io.Writer] instance that it writes to.
type ParquetWriter struct {
	parquet_writer *sfom_parquet.ParquetWriter[*embeddingsdb.Record]
	stats          *Statistics
	mu             *sync.RWMutex
}

// NewWriter returns a new [ParquetWriter] instance configured using 'uri'. If 'uri' is "-"
// then data written (to the writer) will be dispatched to STDOUT. Otherwise 'uri' will be
// treated as the path to a file on the local filesystem.
func NewWriter(ctx context.Context, uri string) (*ParquetWriter, error) {

	parquet_writer, err := sfom_parquet.NewWriter[*embeddingsdb.Record](ctx, uri)

	if err != nil {
		return nil, err
	}

	return newWriter(ctx, parquet_writer)
}

func NewWriterWithIoWriteCloser(ctx context.Context, wr io.WriteCloser) (*ParquetWriter, error) {

	parquet_writer, err := sfom_parquet.NewWriterWithIoWriteCloser[*embeddingsdb.Record](ctx, wr)

	if err != nil {
		return nil, err
	}

	return newWriter(ctx, parquet_writer)
}

func newWriter(ctx context.Context, parquet_writer *sfom_parquet.ParquetWriter[*embeddingsdb.Record]) (*ParquetWriter, error) {

	stats := NewStatistics()
	mu := new(sync.RWMutex)

	pw := &ParquetWriter{
		parquet_writer: parquet_writer,
		stats:          stats,
		mu:             mu,
	}

	return pw, nil
}

// Write will dispatch 'rows' to the underlying Parquet `GenericWriter` instance.
func (pw *ParquetWriter) Write(rows []*embeddingsdb.Record) (int, error) {
	return pw.parquet_writer.Write(rows)
}

// Flush will invoke the  underlying Parquet `GenericWriter` instance's `Flush` method.
func (pw *ParquetWriter) Flush() error {
	return pw.parquet_writer.Flush()
}

// ParquetWriter returns the underlying	Parquet	`GenericWriter`	instance.
func (pw *ParquetWriter) ParquetWriter() *parquet_go.GenericWriter[*embeddingsdb.Record] {
	return pw.parquet_writer.ParquetWriter()
}

// Writer returns the underlying [io.WriteCloser] instance.
func (pw *ParquetWriter) Writer() io.WriteCloser {
	return pw.parquet_writer.Writer()
}

// Close will flush any remaining output and close both the underlying Parquet `GenericWriter`
// and [io.WriteCloser] instances.
func (pw *ParquetWriter) Close() error {

	pw.mu.Lock()
	defer pw.mu.Unlock()

	err := pw.stats.AppendMetadata(pw)

	if err != nil {
		return fmt.Errorf("Failed to append metadata, %w", err)
	}

	return pw.parquet_writer.Close()
}
