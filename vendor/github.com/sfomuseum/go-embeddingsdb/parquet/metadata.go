package parquet

import (
	"fmt"
	"io"

	parquet_go "github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/format"
)

type ReadSeekerAt interface {
	io.ReadSeeker
	io.ReaderAt
}

func KeyValueMetadata(r ReadSeekerAt) (map[string]string, error) {

	meta, err := Metadata(r)

	if err != nil {
		return nil, err
	}

	return KeyValueMetadataFromFileMetaData(meta)
}

func KeyValueMetadataFromFileMetaData(meta *format.FileMetaData) (map[string]string, error) {

	kv_meta := make(map[string]string)

	for _, kv := range meta.KeyValueMetadata {
		kv_meta[kv.Key] = kv.Value
	}

	return kv_meta, nil
}

func Metadata(r ReadSeekerAt) (*format.FileMetaData, error) {

	sz, err := r.Seek(0, io.SeekEnd)

	if err != nil {
		return nil, fmt.Errorf("Failed to derive file size, %w", err)
	}

	_, err = r.Seek(0, 0)

	if err != nil {
		return nil, fmt.Errorf("Failed to rewind file, %w", err)
	}

	f, err := parquet_go.OpenFile(r, sz)

	if err != nil {
		return nil, fmt.Errorf("Failed to derive parquet data, %w", err)
	}

	meta := f.Metadata()
	return meta, nil
}
