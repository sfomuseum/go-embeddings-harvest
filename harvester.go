package harvest

// Work in progress. Much TBD...

import (
	"iter"
	"context"

	"github.com/sfomuseum/go-embeddingsdb"
)

type Harvester interface {
	Iterate(context.Context) iter.Seq2[[]*embeddingsdb.Record, error]
	Close() error
}
