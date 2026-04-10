package oembeddings

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

//go:embed oembeddings.json
var OEmbeddingsJSONSchema []byte

var loadSchema = sync.OnceValues(func() (*jsonschema.Resolved, error) {

	var s jsonschema.Schema

	err := json.Unmarshal(OEmbeddingsJSONSchema, &s)

	if err != nil {
		return nil, fmt.Errorf("Failed to load JSON schema, %w", err)
	}

	resolved, err := s.Resolve(nil)

	if err != nil {
		return nil, fmt.Errorf("Failed to resolve JSON schema, %w", err)
	}

	return resolved, nil
})

// Validate returns a boolean value indicating whether or not the body of 'oe' is a valid OEmbeddings document, conforming to the OEmbeddings JSON schema.
func ValidateWithOEmbeddings(oe *OEmbeddings) (bool, error) {

	body, err := json.Marshal(oe)

	if err != nil {
		return false, fmt.Errorf("Failed to marshal OEmbeddings record, %w", err)
	}

	return Validate(body)
}

// Validate returns a boolean value indicating whether or not a JSON-encoded version of 'attrs' is a valid OEmbeddings document, conforming to the OEmbeddings JSON schema.
func ValidateWithAttributes(attrs map[string]string) (bool, error) {

	enc, err := json.Marshal(attrs)

	if err != nil {
		return false, fmt.Errorf("Failed to marshal attributes, %v", err)
	}

	return Validate(enc)
}

// Validate returns a boolean value indicating whether or not the body of 'r' is a valid OEmbeddings document, conforming to the OEmbeddings JSON schema.
func ValidateWithReader(r io.Reader) (bool, error) {

	body, err := io.ReadAll(r)

	if err != nil {
		return false, fmt.Errorf("Failed to read body, %w", err)
	}

	return Validate(body)
}

// Validate returns a boolean value indicating whether or not 'body' is a valid OEmbeddings document, conforming to the OEmbeddings JSON schema.
func Validate(body []byte) (bool, error) {

	schema, err := loadSchema()

	if err != nil {
		return false, fmt.Errorf("Failed to instatiate JSON schema, %w", err)
	}

	var oe any

	err = json.Unmarshal(body, &oe)

	if err != nil {
		return false, fmt.Errorf("Failed to unmarshal record, %w", err)
	}

	err = schema.Validate(oe)

	if err != nil {
		return false, fmt.Errorf("Failed to validate record, %w", err)
	}

	return true, nil
}
