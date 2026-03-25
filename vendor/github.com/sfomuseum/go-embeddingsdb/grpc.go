package embeddingsdb

import (
	embeddingsdb_grpc "github.com/sfomuseum/go-embeddingsdb/grpc"
)

func EmbeddingsDBRecordToGrpcEmbeddingsDBRecord(record *Record) *embeddingsdb_grpc.EmbeddingsDBRecord {

	grpc_rec := &embeddingsdb_grpc.EmbeddingsDBRecord{
		Provider:    record.Provider,
		DepictionId: record.DepictionId,
		SubjectId:   record.SubjectId,
		Model:       record.Model,
		Attributes:  record.Attributes,
		Embeddings:  record.Embeddings,
		Created:     record.Created,
	}

	return grpc_rec
}

func GrpcEmbeddingsRecordToEmbeddingsDBRecord(grpc_record *embeddingsdb_grpc.EmbeddingsDBRecord) *Record {

	record := &Record{
		Provider:    grpc_record.Provider,
		DepictionId: grpc_record.DepictionId,
		SubjectId:   grpc_record.SubjectId,
		Model:       grpc_record.Model,
		Embeddings:  grpc_record.Embeddings,
		Attributes:  grpc_record.Attributes,
		Created:     grpc_record.Created,
	}

	return record
}

func GrpcSimilarRecordsResultsToEmbeddingDBSimilarRecords(records []*embeddingsdb_grpc.SimilarRecord) []*SimilarRecord {

	count := len(records)
	results := make([]*SimilarRecord, count)

	for idx, rec := range records {

		qr := &SimilarRecord{
			Provider:    rec.Provider,
			DepictionId: rec.DepictionId,
			SubjectId:   rec.SubjectId,
			Distance:    rec.Distance,
			Attributes:  rec.Attributes,
		}

		results[idx] = qr
	}

	return results
}

func EmbeddingsDBSimilarRecordsToGrpcSimilarRecords(results []*SimilarRecord) []*embeddingsdb_grpc.SimilarRecord {

	count := len(results)
	records := make([]*embeddingsdb_grpc.SimilarRecord, count)

	for idx, result := range results {

		records[idx] = &embeddingsdb_grpc.SimilarRecord{
			Provider:    result.Provider,
			DepictionId: result.DepictionId,
			SubjectId:   result.SubjectId,
			Distance:    result.Distance,
			Attributes:  result.Attributes,
		}
	}

	return records
}
