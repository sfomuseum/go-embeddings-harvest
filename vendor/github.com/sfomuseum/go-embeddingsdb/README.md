# go-embeddingsdb

An opinionated Go package for storing, indexing and querying vector embeddings.

## Motivation

There are many vector databases or databases with support for managing vector embeddings. This is not another one. This is, instead, an opinionated Go package for storing, indexing and querying vector embeddings independent of the underlying database using a common interface. Currently efforts are focused on the DuckDB-backed database (using the VSS extension) and a gRPC client/server implementation. The code, as writen, should make it easy enough to support other implementations but those have not been written yet.

This package and the tools it exports still occupy the in-between state of being general purpose and specific to the immediate needs of SFO Museum. That means it may not do what you need it to out of the box. If it doesn't we're certainly open to entertaining changes.

For background, please consult the following blog posts:

* [OEmbeddings - What is the least amount of metadata necessary for shared vector embeddings?](https://millsfield.sfomuseum.org/blog/2026/04/15/oembeddings/), April 2026
* [Shared cross-institutional vector embeddings – how we might get there](https://millsfield.sfomuseum.org/blog/2026/04/06/shared-embeddings/), April 2026
* [Updates (and additions) to machine-learning tools running on consumer hardware](https://millsfield.sfomuseum.org/blog/2026/02/10/docent/), February 2026
* [Similar object images derived using the MobileCLIP computer-vision models](https://millsfield.sfomuseum.org/blog/2026/01/09/similar/), January 2026

## Documentation

At this time `godoc` documentation is incomplete.

## Concepts

There are four principal actors (concepts) to understand with the `go-embeddingsdb`:

1. Records. The inidividual vector embeddings and metadata about the things those embeddings represent.
2. Databases. The place where records are stored, indexed and queried.
3. Servers. Network-based services for interacting with a database.
4. Clients. Tools for interacting with a server.

### Records

Records contain individual embeddings values and related metadata. While not specific to image embeddings they are what most of the work modeling records reflects.

```
// Record defines a struct containing properties associated with individual records stored in an embeddings database.
type Record struct {
	// Provider is the name (or context) of the provider responsible for DepictionId.
	Provider string `json:"provider"`
	// DepictionId is the unique identifier for the depiction for which embeddings have been generated.
	DepictionId string `json:"depiction_id"`
	// SubjectId is the unique identifier associated with the record that DepictionId depicts.
	SubjectId string `json:"subject_id"`
	// Model is the label for the model used to generate embeddings for DepictionId.
	Model string `json:"model"`
	// Embeddings are the embeddings generated for DepictionId using Model.
	Embeddings []float32 `json:"embeddings"`
	// Created is the Unix timestamp when Embeddings were generated.
	Created int64 `json:"created"`
	// Attributes is an arbitrary map of key-value properties associated with the embeddings. Record attributes
	// are encouraged to include the required [OEmbeddings] fields but this is not a requirement.
	Attributes map[string]string `json:"attributes"`
}
```

#### OEmbeddings

_Note: "OEmbeddings" should still be considered work in progress and subject to review and suggestions._

OEmbeddings defines a model for the _least_ amount of metadata to be associated with a vector embedding record in order to allow a preview of the content used to create the embeddings and to display provenance for that content with links back to the subject depicted in the content on a provider's website.

OEmbeddings documentation has been moved in to [oembeddings/README.md](oembeddings/README.md)

### Databases

A database is a system for managing (storing, indexing and querying) embeddings. This package aims to be agnostic to the underlying database system focusing instead on a common interface for use.

```
// Database defines an interface for adding and querying vector embeddings of [embeddingsdb.Record] records.
type Database interface {
	// Return the URI string used to instantiate the Database instance.
	URI() string
	// Add adds a [embeddingsdb.Record] instance to the underlying database implementation. Returns true or false if the addition was batched.
	AddRecord(context.Context, *embeddingsdb.Record, ...options.Option) (bool, error)
	// The number of batched records currently waiting to be added.
	BatchedRecordsCount(context.Context, ...options.Option) (int, error)
	// Add the pending batched records.
	AddBatchedRecords(context.Context, ...options.Option) error
	// Return the EmbeddingsDB instance record matching 'provider', 'depiction_id' and 'model'.
	GetRecord(context.Context, *embeddingsdb.GetRecordRequest, ...options.Option) (*embeddingsdb.Record, error)
	// Remove a record from an EmbeddingsDB instance.
	RemoveRecord(context.Context, *embeddingsdb.RemoveRecordRequest, ...options.Option) error
	// ListRecords returns a paginated list of records stored in the database.
	ListRecords(context.Context, pagination.Options, ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error)
	// IterateRecords returns an [iter.Seq2[*embeddingsdb.Record, error]] for each record stored in the database.
	IterateRecords(context.Context, ...options.Option) iter.Seq2[*embeddingsdb.Record, error]
	// Find similar records for a given model and record instance.
	SimilarRecords(context.Context, *embeddingsdb.SimilarRecordsRequest, ...options.Option) ([]*embeddingsdb.SimilarRecord, error)
	// Export the contents of the database. Where and how a database is exported are left as details for specific implementations.
	Export(context.Context, string, ...options.Option) error
	// Return the Unix timestamp of the last update to the Database instance.
	LastUpdate(context.Context, ...options.Option) (int64, error)
	// Return the list of dimensions supported by this Database  implementation.
	Dimensions(context.Context, ...options.Option) ([]int, error)
	// Return the unique list of models, for zero (all) or more providers, across all the embeddings.
	Models(context.Context, ...options.Option) ([]string, error)
	// Return the unique list of providers across all the embeddings.
	Providers(context.Context, ...options.Option) ([]string, error)
	// Return the pagination type used by the database implementation.
	PaginationType(context.Context, ...options.Option) (PaginationType, error)
	// Close performs and terminating functions required by the database.
	Close(context.Context) error
}
```

### Servers

A server is a network-based service for managing (storing, indexing and querying) embeddings. This package aims to be agnostic to the underlying server semantics focusing instead on a common interface for use.

```
// Server defines an interface for a network-based interface for interacting with an embeddings database.
type Server interface {
	// ListenAndServe starts a new server and listens for requests.
	ListenAndServe(context.Context) error
}
```

### Clients

A client communicates with a server for managing (storing, indexing and querying) embeddings. This package aims to be agnostic to the underlying client semantics focusing instead on a common interface for use.

```
// Client defines an interface for clients to interact with an embeddings database.
type Client interface {
	// Add a new record to an embeddings database.
	AddRecord(context.Context, *embeddingsdb.Record) error
	// Retrieve a specific record from an embeddings database.
	GetRecord(context.Context, *embeddingsdb.GetRecordRequest, ...options.Option) (*embeddingsdb.Record, error)
	// Remove a record from an EmbeddingsDB instance.
	RemoveRecord(context.Context, *embeddingsdb.RemoveRecordRequest, ...options.Option) error
	// ListRecords returns a pagination list of records stored in the database.
	ListRecords(context.Context, pagination.Options, ...options.Option) ([]*embeddingsdb.Record, pagination.Results, error)
	// Retrieve records with similar embeddings from an embeddings database.
	SimilarRecords(context.Context, *embeddingsdb.SimilarRecordsRequest, ...options.Option) ([]*embeddingsdb.SimilarRecord, error)
	// Retrieve records with similar embeddings, for a specific record, from an embeddings database.
	SimilarRecordsById(context.Context, *embeddingsdb.SimilarRecordsByIdRequest, ...options.Option) ([]*embeddingsdb.SimilarRecord, error)
	// Return the unique list of models, for zero (all) or more providers, across all the embeddings.
	Models(context.Context, ...options.Option) ([]string, error)
	// Return the unique list of providers across all the embeddings.
	Providers(context.Context, ...options.Option) ([]string, error)
	// Return the pagination type used by the database implementation.
	PaginationType(context.Context, ...options.Option) (database.PaginationType, error)
	// Close performs and terminating functions required by the client.
	Close(context.Context) error
}
```

## Databases


Database documentation has been moved in to [database/README.md](database/README.md) but here's the "tl;dr".

The DuckDB implementation is generally faster than the SQLite but requires that all your data be stored in memory. That data is periodically exported to disk in order that it may be re-imported without indexing all the data from scratch but it takes a noticeable amount of time to import that data at start up time.

The SQLite implementation while has slower query times but stores (and reads) all its data from disk so it is fast to start.

The Bleve implementation is also fast, has a fast start-up time, doesn't require loading all the data in to memory, doesn't use an unmanageable amount of disk space but remains a non-trivial chore to set up because of the dependency on `libfaiss` (see details in [database/README.md](database/README.md#bleve)). It's also unclear to me whether it is possible to create a single, bundled executable of the Bleve implementation because of the `libfaiss` depedency.

The S3Vectors implementation is fast and demonstrates good query times. It is, however, dependent on a commercial service (Amazon Web Services (AWS)) where everything (from storage to queries) is [metered](https://aws.amazon.com/s3/pricing/?nc=sn&loc=4). Depending on how your database access is configured this could lead to very large bills at the end of the month. If you have already made your peace with AWS then it can be a quick and easy way to get started with vector embeddings.

## Servers

Server documentation has been moved in to [server/README.md](server/README.md)

## Clients

Client documentation has been moved in to [client/README.md](client/README.md)

## Tools

The easiest way to build the included tools is to run the handy `cli` Makefile target (after you've run `go mod tidy && go mod vendor` for reasons described below). For example:

```
$> git clone git@github.com:sfomuseum/go-embeddingsdb.git
$> cd go-embeddingsdb
$> go mod tidy && go mod vendor

$> make cli
go build -tags= -mod vendor -ldflags="-s -w" -o bin/embeddingsdb-client cmd/client/main.go
go build -tags= -mod vendor -ldflags="-s -w" -o bin/embeddingsdb-server cmd/server/main.go
go build -tags= -mod vendor -ldflags="-s -w" -o bin/embeddingsdb-inspector cmd/inspector/main.go
go build -tags= -mod vendor -ldflags="-s -w" -o bin/parquet-export cmd/parquet-export/main.go
go build -tags= -mod vendor -ldflags="-s -w" -o bin/parquet-import cmd/parquet-import/main.go
go build -tags= -mod vendor -ldflags="-s -w" -o bin/parquet-merge cmd/parquet-merge/main.go
```

Tools documentation has been moved in to [cmd/README.md](cmd/README.md)

## Build

What follows are "known knowns", gotchas and other details that may creep when building tools. This gets in to the technical weeds so if that's not your thing you can stop reading now.

### DuckDB

DuckDB is a dependency regardless of build tags (described below).

This package uses the [duckdb/duckdb-go](https://github.com/duckdb/duckdb-go) package for interacting with DuckDB in Go. Although this package bundles all its dependencies in the `vendor` folder there is one notable exception: Any of the `.a` files included in the `duckdb-go` package. That is because it add a couple hundred megabytes to the overall package size. As such you will need to run `go run tidy && go mod vendor` before compiling tools. It's not ideal but it is what it is.

Note: If you need to build a binary tool with support for DuckDB for MacOS _and_ that been signed and notarized you will need to build a customized `libduckdb_bundle.a` from source. See below [for details](#).

### Build tags

Build tags are used to enable support for various features. The default set of tags is empty but you can override those defaults by passing in a custom `TAGS` variable when calling the Makefile targets.

#### bleve

The `bleve` tag adds support for [Bleve](https://blevesearch.com/) document store as an embeddings database. Note that the `vectors` tags is also necessary.

#### no_duckdb

The `no_duckdb` tag disables the availability of DuckDB as a database source. This is mostly so that the `embeddingsdb-inspector` tool can be compiled to run as an AWS Lambda function.

#### sqlite

The `sqlite` tag adds support for the [SQLite](https://sqlite.org/) database as an embeddings database. This uses the [sqlite-vec](https://alexgarcia.xyz/sqlite-vec/) extension for vector embeddings support.

_Note: As of this writing only the Go-language [CGO bindings](https://github.com/asg017/sqlite-vec-go-bindings?tab=readme-ov-file#cgo-bindings) are supported. Support for "pure Go" bindings will be added in future releases._

#### vectors

The `vectors` tag is necessary to compile `libfaiss` code when building Bleve document store support. This is a compliement to the `bleve` tag.

### MacOS

MacOS specific documentation has been moved in to [macos/README.md](macos/README.md).

## See also

* https://github.com/sfomuseum/go-embeddings
* https://github.com/sfomuseum/swift-mobileclip