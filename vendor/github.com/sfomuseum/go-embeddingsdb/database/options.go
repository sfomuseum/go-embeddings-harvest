package database

import (
	"context"
	"fmt"
	"slices"

	"github.com/sfomuseum/go-embeddingsdb/options"
)

func GetMaxDistanceFromOptions(ctx context.Context, opts ...options.Option) *float32 {

	for _, o := range opts {

		if o.Type() == options.MaxDistanceOptionType {
			v := o.(*options.MaxDistanceOption).MaxDistance()
			return &v
		}
	}

	return nil
}

func GetMaxResultsFromOptions(ctx context.Context, opts ...options.Option) *int32 {

	for _, o := range opts {

		if o.Type() == options.MaxResultsOptionType {
			v := o.(*options.MaxResultsOption).MaxResults()
			return &v
		}
	}

	return nil
}

func GetSimilarProviderFromOptions(ctx context.Context, opts ...options.Option) *string {

	for _, o := range opts {

		if o.Type() == options.SimilarProviderOptionType {
			v := o.(*options.SimilarProviderOption).SimilarProvider()
			return &v
		}
	}

	return nil
}

func GetAllProvidersFromOptions(ctx context.Context, opts ...options.Option) []string {

	providers := make([]string, 0)

	for _, o := range opts {

		if o.Type() == options.ProviderOptionType {

			v := o.(*options.ProviderOption).Provider()

			if !slices.Contains(providers, v) {
				providers = append(providers, v)
			}
		}
	}

	return providers
}

func GetDimensionFromOptions(ctx context.Context, opts ...options.Option) (int, error) {

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

func GetAllDimensionsFromOptions(ctx context.Context, opts ...options.Option) []int {

	dimensions := make([]int, 0)

	for _, o := range opts {

		if o.Type() == options.DimensionsOptionType {

			v := o.(*options.DimensionsOption).Dimensions()

			if !slices.Contains(dimensions, v) {
				dimensions = append(dimensions, v)
			}
		}
	}

	return dimensions
}

func GetAllFiltersFromOptions(ctx context.Context, opts ...options.Option) []*options.FilterOption {

	filters := make([]*options.FilterOption, 0)

	for _, o := range opts {

		if o.Type() == options.FilterOptionType {
			f := o.(*options.FilterOption)
			filters = append(filters, f)
		}
	}

	return filters
}
