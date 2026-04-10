//go:build sqlite

package database

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"
)

const matroyshka_dimensions int = 512

const sqlite_vec_default_compression string = "none"
const sqlite_vec_quantize_compression string = "quantize"
const sqlite_vec_matroyshka_compression string = "matroyshka"

var sqlite_vec_compressions = []string{
	sqlite_vec_default_compression,
	sqlite_vec_quantize_compression,
	sqlite_vec_matroyshka_compression,
}

func IsValidSQLiteCompression(c string) bool {
	return slices.Contains(sqlite_vec_compressions, c)
}

// Compliment method to SerializeFloat32
// https://github.com/asg017/sqlite-vec-go-bindings/blob/main/cgo/lib.go#L33

func DeserializeFloat32(b []byte) ([]float32, error) {

	if len(b)%4 != 0 {
		return nil, fmt.Errorf("byte slice length %d is not a multiple of 4", len(b))
	}

	n := len(b) / 4           // number of float32 values
	vec := make([]float32, n) // allocate destination slice

	buf := bytes.NewReader(b)

	// binary.Read will read n float32 values into vec
	if err := binary.Read(buf, binary.LittleEndian, vec); err != nil {
		return nil, err
	}
	return vec, nil
}

func DeserializeQuantizedBinary(data []byte) []float32 {

	// https://alexgarcia.xyz/sqlite-vec/guides/binary-quant.html

	dims := len(data) * 8
	unpacked := make([]float32, dims)

	for i, b := range data {
		for j := range 8 {
			if (b & (1 << (7 - j))) != 0 {
				unpacked[i*8+j] = 1.0
			} else {
				unpacked[i*8+j] = -1.0
			}
		}
	}

	return unpacked
}
