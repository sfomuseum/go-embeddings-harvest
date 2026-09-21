package sqlite

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"slices"
)

const VectorMatroyshkaDimensions int = 512

const VectorDefaultCompression string = "none"
const VectorQuantizeCompression string = "quantize"
const VectorMatroyshkaCompression string = "matroyshka"

var VectorCompressions = []string{
	VectorDefaultCompression,
	VectorQuantizeCompression,
	VectorMatroyshkaCompression,
}

func IsValidVectorCompression(c string) bool {
	return slices.Contains(VectorCompressions, c)
}

// Copied from (because not exported in modernc/sqlite/vec):
// https://github.com/asg017/sqlite-vec-go-bindings/blob/main/cgo/lib.go#L33

// Serializes a float32 list into a vector BLOB that sqlite-vec accepts.
func SerializeFloat32Vec(vector []float32) ([]byte, error) {

	buf := new(bytes.Buffer)

	err := binary.Write(buf, binary.LittleEndian, vector)

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Compliment method to SerializeFloat32
// https://github.com/asg017/sqlite-vec-go-bindings/blob/main/cgo/lib.go#L33

func DeserializeFloat32Vec(b []byte) ([]float32, error) {

	if len(b)%4 != 0 {
		return nil, fmt.Errorf("byte slice length %d is not a multiple of 4", len(b))
	}

	n := len(b) / 4           // number of float32 values
	vec := make([]float32, n) // allocate destination slice

	buf := bytes.NewReader(b)

	// binary.Read will read n float32 values into vec
	err := binary.Read(buf, binary.LittleEndian, vec)

	if err != nil {
		return nil, err
	}
	
	return vec, nil
}

func DeserializeQuantizedBinaryVec(data []byte) []float32 {

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
