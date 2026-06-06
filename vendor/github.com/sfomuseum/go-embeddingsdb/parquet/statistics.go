package parquet

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/sfomuseum/go-embeddingsdb"
)

// Statistics holds aggregated information about a collection of
// embeddingsdb.Record entries.
type Statistics struct {
	// Models is the list of distinct model names that have been seen.
	Models []string `json:"models"`
	// Providers is the list of distinct provider names that have been seen.
	Providers []string `json:"providers"`
	// ModelProviders maps a model name to the list of providers that have
	// produced embeddings for that model.
	ModelProviders map[string][]string `json:"model_providers"`
	// ModelDimensions maps a model name to the dimensionality of its embeddings.
	ModelDimensions map[string]int `json:"model_dimensions"`
	// ProviderModels maps a provider name to the list of models that have
	// produced embeddings for that provider.
	ProviderModels map[string][]string `json:"provider_models"`
	mu             *sync.RWMutex
}

// NewStatistics creates a new, empty Statistics instance.  The returned
// value is safe for concurrent use.
func NewStatistics() *Statistics {

	models := make([]string, 0)
	providers := make([]string, 0)

	model_providers := make(map[string][]string)
	model_dimensions := make(map[string]int)
	provider_models := make(map[string][]string)

	mu := new(sync.RWMutex)

	s := &Statistics{
		Models:          models,
		Providers:       providers,
		ModelProviders:  model_providers,
		ModelDimensions: model_dimensions,
		ProviderModels:  provider_models,
		mu:              mu,
	}

	return s
}

// AddRecord updates the Statistics with information from a single record.
func (s *Statistics) AddRecord(rec *embeddingsdb.Record) error {

	s.mu.Lock()

	if !slices.Contains(s.Models, rec.Model) {
		s.Models = append(s.Models, rec.Model)
	}

	if !slices.Contains(s.Providers, rec.Provider) {
		s.Providers = append(s.Providers, rec.Provider)
	}

	model_providers, exists := s.ModelProviders[rec.Model]

	if !exists {
		model_providers = make([]string, 0)
	}

	if !slices.Contains(model_providers, rec.Provider) {
		model_providers = append(model_providers, rec.Provider)
	}

	s.ModelProviders[rec.Model] = model_providers

	_, exists = s.ModelDimensions[rec.Model]

	if !exists {
		s.ModelDimensions[rec.Model] = len(rec.Embeddings)
	}

	provider_models, exists := s.ProviderModels[rec.Provider]

	if !exists {
		provider_models = make([]string, 0)
	}

	if !slices.Contains(provider_models, rec.Model) {
		provider_models = append(provider_models, rec.Model)
	}

	s.ProviderModels[rec.Provider] = provider_models

	s.mu.Unlock()
	return nil
}

// AppendMetadata writes the collected statistics to the supplied ParquetWriter
// as key/value metadata.  The keys are namespaced with the prefix "embeddingsdb:"
// to avoid collisions with other writers.
func (s *Statistics) AppendMetadata(wr *ParquetWriter) error {

	s.mu.Lock()

	p_wr := wr.ParquetWriter()

	p_wr.SetKeyValueMetadata("embeddingsdb:models", strings.Join(s.Models, ";"))
	p_wr.SetKeyValueMetadata("embeddingsdb:providers", strings.Join(s.Providers, ";"))

	for k, v := range s.ModelDimensions {
		p_wr.SetKeyValueMetadata(fmt.Sprintf("embeddingsdb:model:%s:dimensions", k), strconv.Itoa(v))
	}

	for k, v := range s.ModelProviders {
		p_wr.SetKeyValueMetadata(fmt.Sprintf("embeddingsdb:model:%s:providers", k), strings.Join(v, ";"))
	}

	for k, v := range s.ProviderModels {
		p_wr.SetKeyValueMetadata(fmt.Sprintf("embeddingsdb:provider:%s:models", k), strings.Join(v, ";"))
	}

	s.mu.Unlock()
	return nil
}

// GatherStatistics walks through one or more Parquet files and builds a
// Statistics instance that summarises all records found.
func GatherStatistics(ctx context.Context, uris ...string) (*Statistics, error) {

	stats := NewStatistics()

	for _, uri := range uris {

		for row, err := range Iterate(ctx, uri) {

			if err != nil {
				return nil, err
			}

			// There isn't much point in doing this concurrently since
			// stats will lock access anyway...

			stats.AddRecord(row)
		}
	}

	return stats, nil
}
