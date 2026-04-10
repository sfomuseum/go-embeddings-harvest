package embeddingsdb

import (
	"github.com/sfomuseum/go-embeddingsdb/oembeddings"
)

type GetRecordRequest struct {
	// Provider is the name (or context) of the provider responsible for DepictionId.
	Provider string `json:"provider"`
	// Model is the name of the model to specify when querying for similar embeddings.
	Model string `json:"model"`
	// DepictionId is the unique identifier for the depiction for which embeddings have been generated.
	DepictionId string `json:"depiction_id"`
}

type GetRecordResponse struct {
	Record *Record `json:"record"`
}

type RemoveRecordRequest struct {
	// Provider is the name (or context) of the provider responsible for DepictionId.
	Provider string `json:"provider"`
	// Model is the name of the model to specify when querying for similar embeddings.
	Model string `json:"model"`
	// DepictionId is the unique identifier for the depiction for which embeddings have been generated.
	DepictionId string `json:"depiction_id"`
}

type SimilarRecordsByIdRequest struct {
	// Provider is the name (or context) of the provider responsible for DepictionId.
	Provider string `json:"provider"`
	// Model is the name of the model to specify when querying for similar embeddings.
	Model string `json:"model"`
	// DepictionId is the unique identifier for the depiction for which embeddings have been generated.
	DepictionId string `json:"depiction_id"`
	// The name of the provider to limit similar record queries to. If empty then all the records for the model chosen will be queried.
	SimilarProvider *string `json:"similar_provider,omitempty"`
	// ...
	MaxDistance *float32 `json:"max_distance,omitempty"`
	// ...
	MaxResults *int32 `json:"max_results,omitempty"`
}

// SimilarRecordsRequest is a struct containing properties for retrieving records from an embeddings database.
type SimilarRecordsRequest struct {
	// Model is the name of the model to specify when querying for similar embeddings.
	Model string `json:"model"`
	// Embeddings are the embeddings to use for querying similar records.
	Embeddings []float32 `json:"embeddings"`
	// Zero or more depiction IDs to explicitly exclude from results.
	Exclude []string `json:"exclude,omitempty"`
	// The name of the provider to limit similar record queries to. If empty then all the records for the model chosen will be queried.
	SimilarProvider *string `json:"similar_provider,omitempty"`
	// ...
	MaxDistance *float32 `json:"max_distance,omitempty"`
	// ...
	MaxResults *int32 `json:"max_results,omitempty"`
}

type SimilarRecordResponse struct {
	Records []*SimilarRecord `json:"records"`
}

// SimilarRecord is a struct containing properties for similar records returned from an embeddings database.
type SimilarRecord struct {
	// Provider is the name (or context) of the provider responsible for DepictionId.
	Provider string `json:"provider"`
	// DepictionID is the unique identifier	for the	depiction associated with the result.
	DepictionId string `json:"depiction_id"`
	// DepictionID is the unique identifier	for the	subject associated with the result depiction.
	SubjectId string `json:"subject_id"`
	// Distance is the distance between the input embeddings and the result embeddings.
	Distance float32 `json:"similarity"`
	// Attributes is an arbitrary map of key-value properties associated with the embeddings.
	Attributes map[string]string `json:"attributes"`
}

// Derive an [oembeddings.OEmbeddings] instance from the attributes in 'r'.
func (r *SimilarRecord) OEmbeddings() (*oembeddings.OEmbeddings, error) {
	return oembeddings.FromAttributes(r.Attributes)
}

// Derive an [oembeddings.OEmbeddings] instance from the attributes in 'r'. Return nil if this is not possible.
func (r *SimilarRecord) OEmbeddingsOrNil() *oembeddings.OEmbeddings {
	oe, _ := oembeddings.FromAttributes(r.Attributes)
	return oe
}
