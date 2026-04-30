package database

import (
	"context"

	"github.com/sfomuseum/go-embeddingsdb/models"
	"github.com/sfomuseum/go-embeddingsdb/options"
)

func DeriveModelDimensions(ctx context.Context, model string, opts ...options.Option) (int, error) {

	d, exists := models.DeriveDimensionsFromModel(model)

	if exists {
		return d, nil
	}

	return GetDimensionFromOptions(ctx, opts...)
}
