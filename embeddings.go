package harvest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/sfomuseum/go-embeddings"
	"github.com/sfomuseum/go-embeddingsdb"
)

// DeriveEmbeddingsRecordsOptions specifies the parameters, payload metadata,
// and explicit models required to transform raw asset data into vector database records.
type DeriveEmbeddingsRecordsOptions struct {
	// Provider is the name or domain identifier of the data source.
	Provider string
	// DepictionId is the unique identifier for the specific asset representation (e.g., image ID).
	DepictionId string
	// SubjectId is the internal tracking identifier for the artwork or physical object.
	SubjectId string
	// Attributes contains structured metadata key-value properties describing the record. It is expected
	// to conform to the `sfomuseum/go-embeddingsdb/oembeddings.OEmbeddings` model.
	Attributes map[string]string
	// Models maps to the individual model naming identifiers targeted for processing.
	Models []string
	// Body contains the raw byte array stream of the object representation (e.g., image bytes).
	Body []byte
}

// DeriveEmbeddingsRecords coordinates the execution lifecycle to query an embedding client
// and maps responses to records. It automatically optimizes processing strategies by adapting
// to sequential single executions or multi-threaded concurrent routines based on model length.
func DeriveEmbeddingsRecords(ctx context.Context, cl embeddings.Embedder[float32], opts *DeriveEmbeddingsRecordsOptions) ([]*embeddingsdb.Record, error) {

	logger := slog.Default()
	logger = logger.With("depiction", opts.DepictionId)

	t1 := time.Now()

	defer func() {
		logger.Debug("Time to derive all embeddings", "time", time.Since(t1))
	}()

	records := make([]*embeddingsdb.Record, 0)

	switch len(opts.Models) {
	case 0:

		db_rec, err := deriveEmbeddingsWithModel(ctx, cl, opts, "")

		if err != nil {
			logger.Error("Failed to derive embeddings", "error", err)
		} else {
			records = []*embeddingsdb.Record{db_rec}
		}

	case 1:

		db_rec, err := deriveEmbeddingsWithModel(ctx, cl, opts, opts.Models[0])

		if err != nil {
			logger.Error("Failed to derive embeddings", "model", opts.Models[0], "error", err)
		} else {
			records = []*embeddingsdb.Record{db_rec}
		}

	default:

		wg := new(sync.WaitGroup)
		mu := new(sync.Mutex)

		for _, m := range opts.Models {

			wg.Go(func() {

				db_rec, err := deriveEmbeddingsWithModel(ctx, cl, opts, m)

				if err != nil {
					logger.Error("Failed to derive embeddings", "model", m, "error", err)
					return
				}

				mu.Lock()
				records = append(records, db_rec)
				mu.Unlock()
			})
		}

		wg.Wait()
	}

	return records, nil
}

// deriveEmbeddingsWithModel interacts directly with the downstream Embedder client using a single
// specified model, verifies response validity, and packs the payload into an embeddingsdb.Record.
func deriveEmbeddingsWithModel(ctx context.Context, cl embeddings.Embedder[float32], opts *DeriveEmbeddingsRecordsOptions, model string) (*embeddingsdb.Record, error) {

	logger := slog.Default()
	logger = logger.With("depiction", opts.DepictionId)
	logger = logger.With("model", model)

	t1 := time.Now()

	defer func() {
		logger.Debug("Time to derive all embeddings", "time", time.Since(t1))
	}()

	emb_req := &embeddings.EmbeddingsRequest{
		Model: model,
		Body:  opts.Body,
	}

	logger.Info("Get image embeddings...")

	emb_rsp, err := cl.ImageEmbeddings(ctx, emb_req)

	if err != nil {
		logger.Error("Failed to derive embeddings", "error", err)
		return nil, err
	}

	if len(emb_rsp.Embeddings()) == 0 {
		logger.Error("Zero-length embeddings", "error", err)
		return nil, err
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

	return db_rec, nil
}
