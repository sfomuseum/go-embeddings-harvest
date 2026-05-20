package options

import (
	"context"
	"fmt"
	"slices"
)

func GetFilterFromOptions(ctx context.Context, key string, opts ...Option) any {

	for _, o := range opts {

		if o.Type() == FilterOptionType {
			f := o.(*FilterOption)

			if f.Key() == key {
				return f.Value()
			}
		}
	}

	return nil
}

func GetModelFromOptions(ctx context.Context, opts ...Option) *string {

	for _, o := range opts {

		if o.Type() == ModelOptionType {
			v := o.(*ModelOption).Model()
			return &v
		}
	}

	return nil
}

func GetProviderFromOptions(ctx context.Context, opts ...Option) *string {

	for _, o := range opts {

		if o.Type() == ProviderOptionType {
			v := o.(*ProviderOption).Provider()
			return &v
		}
	}

	return nil
}

func GetMaxDistanceFromOptions(ctx context.Context, opts ...Option) *float32 {

	for _, o := range opts {

		if o.Type() == MaxDistanceOptionType {
			v := o.(*MaxDistanceOption).MaxDistance()
			return &v
		}
	}

	return nil
}

func GetMaxResultsFromOptions(ctx context.Context, opts ...Option) *int32 {

	for _, o := range opts {

		if o.Type() == MaxResultsOptionType {
			v := o.(*MaxResultsOption).MaxResults()
			return &v
		}
	}

	return nil
}

func GetSimilarProviderFromOptions(ctx context.Context, opts ...Option) *string {

	for _, o := range opts {

		if o.Type() == SimilarProviderOptionType {
			v := o.(*SimilarProviderOption).SimilarProvider()
			return &v
		}
	}

	return nil
}

func GetAllProvidersFromOptions(ctx context.Context, opts ...Option) []string {

	providers := make([]string, 0)

	for _, o := range opts {

		if o.Type() == ProviderOptionType {

			v := o.(*ProviderOption).Provider()

			if !slices.Contains(providers, v) {
				providers = append(providers, v)
			}
		}
	}

	return providers
}

func GetAllModelsFromOptions(ctx context.Context, opts ...Option) []string {

	models := make([]string, 0)

	for _, o := range opts {

		if o.Type() == ModelOptionType {

			v := o.(*ModelOption).Model()

			if !slices.Contains(models, v) {
				models = append(models, v)
			}
		}
	}

	return models
}

func GetDimensionFromOptions(ctx context.Context, opts ...Option) (int, error) {

	dims := GetAllDimensionsFromOptions(ctx)
	count := len(dims)

	switch {
	case count == 0:
		return 0, fmt.Errorf("Missing dimensions option")
	case count > 1:
		return 0, fmt.Errorf("Multiple dimensions specified")
	default:
		return dims[0], nil
	}

}

func GetAllDimensionsFromOptions(ctx context.Context, opts ...Option) []int {

	dimensions := make([]int, 0)

	for _, o := range opts {

		if o.Type() == DimensionsOptionType {

			v := o.(*DimensionsOption).Dimensions()

			if !slices.Contains(dimensions, v) {
				dimensions = append(dimensions, v)
			}
		}
	}

	return dimensions
}

func GetAllFiltersFromOptions(ctx context.Context, opts ...Option) []*FilterOption {

	filters := make([]*FilterOption, 0)

	for _, o := range opts {

		if o.Type() == FilterOptionType {
			f := o.(*FilterOption)
			filters = append(filters, f)
		}
	}

	return filters
}
