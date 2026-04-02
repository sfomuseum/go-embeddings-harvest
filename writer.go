package harvest

import (
	"fmt"
	"io"
	"os"

	parquet_go "github.com/parquet-go/parquet-go"
	"github.com/sfomuseum/go-embeddingsdb"
)

// nopWriteCloser is an io.WriteCloser that does nothing on Close().
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// NopWriteCloser returns an io.WriteCloser that wraps w and whose Close() is a no‑op.
func NopWriteCloser(w io.Writer) io.WriteCloser {
	return nopWriteCloser{w}
}

type HarvestWriter struct {
	wr            io.WriteCloser
	ParquetWriter *parquet_go.GenericWriter[*embeddingsdb.Record]
}

func NewWriter(output string) (*HarvestWriter, error) {

	var wr io.WriteCloser

	switch output {
	case "-":
		wr = NopWriteCloser(os.Stdout)
	default:

		w, err := os.OpenFile(output, os.O_RDWR|os.O_CREATE, 0644)

		if err != nil {
			return nil, fmt.Errorf("Failed to open %s for writing, %w", output, err)
		}

		wr = w
	}

	p_wr := parquet_go.NewGenericWriter[*embeddingsdb.Record](wr)

	h_wr := &HarvestWriter{
		wr:            wr,
		ParquetWriter: p_wr,
	}

	return h_wr, nil
}

func (h *HarvestWriter) Close() error {

	h.ParquetWriter.Flush()

	err := h.ParquetWriter.Close()

	if err != nil {
		return fmt.Errorf("Failed to close Parquet writer, %w", err)
	}

	err = h.wr.Close()

	if err != nil {
		return fmt.Errorf("Failed to close writer, %w", err)
	}

	return nil
}
