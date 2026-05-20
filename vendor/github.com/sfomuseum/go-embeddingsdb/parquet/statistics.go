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

type Statistics struct {
	Models          []string            `json:"models"`
	Providers       []string            `json:"providers"`
	ModelProviders  map[string][]string `json:"model_providers"`
	ModelDimensions map[string]int      `json:"model_dimensions"`
	ProviderModels  map[string][]string `json:"provider_models"`
	mu              *sync.RWMutex
}

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

func (s *Statistics) AddRecord(r any) error {

	var rec *embeddingsdb.Record
	switch r.(type) {
	case *embeddingsdb.Record:
		rec = r.(*embeddingsdb.Record)
	default:
		return fmt.Errorf("Invalid record type")
	}

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

func (s *Statistics) AppendMetadata(wr *ParquetWriter) error {

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

	return nil
}

func GatherStatistics(ctx context.Context, uris ...string) (*Statistics, error) {

	stats := NewStatistics()

	for _, uri := range uris {

		for row, err := range Iterate(ctx, uri) {

			if err != nil {
				return nil, err
			}

			stats.AddRecord(row)
		}
	}

	return stats, nil
}
