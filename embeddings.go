package harvest

import (
	"context"
	"log/slog"
	"sync"

	"github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddingsdb"
)

type DeriveEmbeddingsRecordsOptions struct {
	Provider         string
	DepictionId      string
	SubjectId        string
	Attributes       map[string]string
	Models           []string
	Body             []byte
}

func DeriveEmbeddingsRecords(ctx context.Context, cl embeddings.Embedder[float32], opts *DeriveEmbeddingsRecordsOptions) ([]*embeddingsdb.Record, error) {

	logger := slog.Default()
	logger = logger.With("depiction", opts.DepictionId)

	records := make([]*embeddingsdb.Record, 0)

	wg := new(sync.WaitGroup)
	mu := new(sync.RWMutex)

	for _, m := range opts.Models {

		wg.Go(func() {

			emb_req := &embeddings.EmbeddingsRequest{
				Model: m,
				Body:  opts.Body,
			}

			emb_rsp, err := cl.ImageEmbeddings(ctx, emb_req)

			if err != nil {
				logger.Error("Failed to derive embeddings", "model", m, "error", err)
				return
			}

			db_rec := &embeddingsdb.Record{
				Provider:    opts.Provider,
				DepictionId: opts.DepictionId,
				SubjectId:   opts.SubjectId,
				Model:       emb_rsp.Model(),
				Embeddings:  emb_rsp.Embeddings(),
				Attributes:  opts.Attributes,
				Created:     emb_rsp.Created(),
			}

			logger.Debug("Add record", "key", db_rec.Key())

			mu.Lock()
			records = append(records, db_rec)
			mu.Unlock()
		})
	}

	wg.Wait()

	return records, nil
}
