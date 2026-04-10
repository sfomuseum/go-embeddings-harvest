package embeddingsdb

import (
	"fmt"

	"github.com/sfomuseum/go-embeddingsdb/oembeddings"
)

// Record defines a struct containing properties associated with individual records stored in an embeddings database.
type Record struct {
	// Provider is the name (or context) of the provider responsible for DepictionId.
	Provider string `json:"provider" parquet:"provider,dict,zstd"`
	// DepictionId is the unique identifier for the depiction for which embeddings have been generated.
	DepictionId string `json:"depiction_id" parquet:"depiction_id,dict,zstd"`
	// SubjectId is the unique identifier associated with the record that DepictionId depicts.
	SubjectId string `json:"subject_id" parquet:"subject_id,dict,zstd"`
	// Model is the label for the model used to generate embeddings for DepictionId.
	Model string `json:"model" parquet:"model,dict,zstd"`
	// Embeddings are the embeddings generated for DepictionId using Model.
	Embeddings []float32 `json:"embeddings" parquet:"embeddings,plain,list"	// Note the 'plain,list'. This is important in order to prevent DLBA which makes DuckDB sad.`
	// Created is the Unix timestamp when Embeddings were generated.
	Created int64 `json:"created" parquet:"created"`
	// Attributes is an arbitrary map of key-value properties associated with the embeddings. Record attributes
	// are encouraged to include the required [OEmbeddings] fields but this is not a requirement.
	Attributes map[string]string `json:"attributes" parquet:"attributes"`
}

func (r *Record) Key() string {
	return fmt.Sprintf("%s-%s-%s", r.Provider, r.DepictionId, r.Model)
}

// Derive an [oembeddings.OEmbeddings] instance from the attributes in 'r'.
func (r *Record) OEmbeddings() (*oembeddings.OEmbeddings, error) {
	return oembeddings.FromAttributes(r.Attributes)
}

// Derive an [oembeddings.OEmbeddings] instance from the attributes in 'r'. Return nil if this is not possible.
func (r *Record) OEmbeddingsOrNil() *oembeddings.OEmbeddings {
	oe, _ := oembeddings.FromAttributes(r.Attributes)
	return oe
}
