package harvest

import (
	"context"
	"iter"

	"github.com/sfomuseum/go-embeddingsdb"
)

func init() {
	MustRegisterHarvester(context.Background(), "null", NewNullHarvester)
}

type NullHarvester struct {
	Harvester
	path_objects string
}

func NewNullHarvester(ctx context.Context, uri string) (Harvester, error) {

	h := &NullHarvester{}
	return h, nil
}

func (h *NullHarvester) Iterate(ctx context.Context, opts *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {
		return
	}
}

func (h *NullHarvester) Close() error {
	return nil
}
