package models

import (
	_ "embed"
	"encoding/json"
	"sync"
)

//go:embed dimensions.json
var dimensions_json []byte

type lookup_table map[string]int

var dimensions = sync.OnceValues(func() (map[string]int, error) {

	var lookup map[string]int

	err := json.Unmarshal(dimensions_json, &lookup)
	return lookup, err
})

func KnownDimensions() (map[string]int, error) {
	return dimensions()
}

func DeriveDimensionsFromModel(model string) (int, bool) {

	lookup, err := dimensions()

	if err != nil {
		return 0, false
	}

	d, exists := lookup[model]
	return d, exists
}
