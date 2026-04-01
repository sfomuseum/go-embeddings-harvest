# go-embeddings-harvest

Go package for harvesting data from a variety of providers, deriving vector embeddings for those data and writing everything to Parquet files.

## Motivation

The broader aim is to try and establish what the "simplest and dumbest" amount of metadata is necessary for two or more cultural heritage institutions to share vector embedding data, of their respective collections or holdings, in order to perform cross-institutional similarity queries.

This package provides tools for generating those "shareable" data as Parquet files. These Parquet files encode rows which map to the `sfomuseum/go-embeddingsdb.Record` data model which looks like this:

```
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
	Embeddings []float32 `json:"embeddings" parquet:"embeddings,list"`
	// Created is the Unix timestamp when Embeddings were generated.
	Created int64 `json:"created" parquet:"created"`
	// Attributes is an arbitrary map of key-value properties associated with the embeddings.
	Attributes map[string]string `json:"attributes" parquet:"attributes"`
}
```

Currently this work targets vector embeddings for images of collection objects, or "depictions" of "subjects" respectively. These are assumed to be the internal unique identifiers assigned by the institution (or "provider") responsible for those objects, and their images.

There are no rules, or even conventions, for how to identify "providers". A fully-qualified URL would be an obvious choice but it introduces a lot repeated boiler-plate in to the Parquet files. Maybe that doesn't matter.

Likewise, there are not conventions for what should be included in the `Attributes` property which is currently defined as a freeform key-value lookup. So far the only convention has been to include a link to the image, or a thumbnail of that image, used to generate embeddings so that it is possible to useful inspect the results of a similary query against a set of embeddings. Should there be others required properties, though? For example:

* Title?
* A link back to an online representation of the subject (or the depiction)?
* Perhaps a link to a IIIF manifest or some other machine-readable metadata?
* Is it okay for multiple depictions to point back to a machine-readable document referencing the subject? Current practice rarely assumes machine-readable representations of depiction (image) assets.

The goal here is the establish the _least amount of metadata_ necessary to accurately reflect provenance and to provide avenues for machine-readable metadata to be derived on a case-by-case basis.

So the purpose of this package is to provide tools to generate Parquet-encoded representations of those data for a variety of sources to help work through those questions.

_Eventually embeddings for text-based sources will be supported but that hasn't happened yet._

## Tools

```
$> make cli
go build -mod vendor -ldflags="-s -w" -o bin/harvest-flickr-embeddings cmd/harvest-flickr-embeddings/main.go
go build -mod vendor -ldflags="-s -w" -o bin/harvest-nga-embeddings cmd/harvest-nga-embeddings/main.go
go build -mod vendor -ldflags="-s -w" -o bin/harvest-sfomuseum-media-embeddings cmd/harvest-sfomuseum-media-embeddings/main.go
go build -mod vendor -ldflags="-s -w" -o bin/harvest-sfomuseum-instagram-embeddings cmd/harvest-sfomuseum-instagram-embeddings/main.go
```

### harvest-flickr-embeddings

For example:

```
./bin/harvest-flickr-embeddings \
	-flickr-client-uri file:///usr/local/flickr.txt \
	-param user_id=49487266@N07 \
	-param method=flickr.photosets.getPhotos \
	-param photoset_id=flickr.photosets.getPhotos \
	-provider flickr-49487266@N07 \
	-spr-path photoset.photo \
	-model s0,s1,s2 \
	-output flickr.parquet \
	-verbose	
```

### harvest-nga-embeddings

### harvest-sfomuseum-instagram-embeddings

### harvest-sfomuseum-media-embeddings

## See also

* https://github.com/sfomuseum/go-embeddings
* https://github.com/sfomuseum/go-embeddingsdb