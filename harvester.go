package harvest

// Work in progress. Much TBD...

import (
	"iter"
	"context"

	"github.com/sfomuseum/go-blobcache/http"	
	"github.com/sfomuseum/go-embeddings"	
	"github.com/sfomuseum/go-embeddingsdb"
)

type Harvester interface {
	Iterate(context.Context, embeddings.Embedder, *http.GetWithCacheOptions) iter.Seq2[[]*embeddingsdb.Record, error]
	Close() error
}
