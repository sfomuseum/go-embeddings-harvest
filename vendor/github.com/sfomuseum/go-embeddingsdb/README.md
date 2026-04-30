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

The Bleve implementation is also fast, has a fast start-up time, doesn't require loading all the data in to memory, doesn't use an unmanageable amount of disk space but remains a non-trivial chore to set up because of the dependency on `libfaiss` (see details below). It's also unclear to me whether it is possible to create a single, bundled executable of the Bleve implementation because of the `libfaiss` depedency.

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

#### statically linked DuckDB extensions

If you want to build a `emeddingsdb-server` binary (or any other tool that uses this package as a library) for MacOS with support for DuckDB _and_ that has been signed and notarized you will need to compile a custom `libduckdb_bundle.a` library with both the JSON and VSS extensions statically linked. Then you will need to use specify that custom library when building the `emeddingsdb-server` binary. This is because the default behaviour for DuckDB is to load (and cache) extensions on the fly and those extensions will have been signed by someone other than the "team" (you) that notarized the `emeddingsdb-server` binary.

After a fair amount of trial and error this is what I managed to get working. It _should_ work for you but you know how these things end up changing when you're not looking.

_Note: There are [known problems with this process using recent releases of DuckDB](https://github.com/duckdb/duckdb-spatial/issues/794). I am trying to figure them out._

First install both `duckdb` and `vcpkg` from source:

```
$> git clone https://github.com/duckdb/duckdb.git /usr/local/src/duckdb
$> git clone https://github.com/microsoft/vcpkg.git /usr/local/src/vcpkg

$> cd /usr/local/src/duckdb
```

Now copy the `vss.cmake` config file in to the root directory:

```
$> cp .github/config/extensions/vss.cmake ./vss_config.cmake
```

Now edit it to remove the `DONT_LINK` instruction. For example:

```
duckdb_extension_load(vss
        LOAD_TESTS
        GIT_URL https://github.com/duckdb/duckdb-vss
        GIT_TAG c8a4efe05003d8ef6eaad34f5521cf50126c9967
        TEST_DIR test/sql
        APPLY_PATCHES
    )
```

Ensure the following environment variables are set:

```
$> printenv

GEN=ninja
BUILD_VSS=1
BUILD_JSON=1
EXTENSION_CONFIGS=vss_config.cmake
VCPKG_TOOLCHAIN_PATH=/usr/local/src/vcpkg/scripts/buildsystems/vcpkg.cmake
VCPKG_ROOT=/usr/local/src/vcpkg
```

Note the use of the `BUILD_JSON` environment variable. This will bundle the JSON extension which is necessary to use the VSS extension.

Now build the command line tool so you can verify that the VSS (and JSON) extensions are statically linked:

```
$> make

... stuff happens

$> du -h /usr/local/src/duckdb/build/release/duckdb
 43M	/usr/local/src/duckdb/build/release/duckdb
```

Once built, check the installed (and loaded) extensions:

```
$> /usr/local/src/duckdb/build/release/duckdb

DuckDB v1.5.0-dev5476 (Development Version, 1c62e11b82)
Enter ".help" for usage hints.

memory D SELECT extension_name, loaded, installed, install_mode FROM duckdb_extensions() WHERE installed = true;
┌────────────────┬─────────┬───────────┬───────────────────┐
│ extension_name │ loaded  │ installed │   install_mode    │
│    varchar     │ boolean │  boolean  │      varchar      │
├────────────────┼─────────┼───────────┼───────────────────┤
│ core_functions │ true    │ true      │ STATICALLY_LINKED │
│ json           │ true    │ true      │ STATICALLY_LINKED │
│ parquet        │ true    │ true      │ STATICALLY_LINKED │
│ shell          │ true    │ true      │ STATICALLY_LINKED │
│ vss            │ true    │ true      │ STATICALLY_LINKED │
└────────────────┴─────────┴───────────┴───────────────────┘
```

Assuming that the `vss` extension is installed and loaded build DuckDB again as a library:

```
$> make bundle-library

... stuff happens

$> du -h /usr/local/src/duckdb/build/release/libduckdb_bundle.a
 79M	/usr/local/src/duckdb/build/release/libduckdb_bundle.a
```

Apply additional MacOS hoop-jumping, appending the `generated_extension_loader.cpp.o` file to the `libduckdb_bundle.a` file::

```
$> find /usr/local/src/duckdb/build/release -name "generated_extension_loader.cpp.o"
/usr/local/src/duckdb/build/release/extension/CMakeFiles/duckdb_generated_extension_loader.dir/__/codegen/src/generated_extension_loader.cpp.o

$> ar rcs /usr/local/src/duckdb/build/release/libduckdb_bundle.a /usr/local/src/duckdb/build/release/extension/CMakeFiles/duckdb_generated_extension_loader.dir/__/codegen/src/generated_extension_loader.cpp.o
```

Finally rebuild the `embeddingsdb-server` with the customized DuckDB library using the handy `server-bundle` Makefile target (in this repo):

```
$> cd /usr/local/src/go-embeddingsdb
$> mkdir work
$> cp /usr/local/src/duckdb/build/release/libduckdb_bundle.a ./work/

$> make server-bundle
CGO_ENABLED=1 CPPFLAGS="-DDUCKDB_STATIC_BUILD" CGO_LDFLAGS="-L./work -lduckdb_bundle -lc++" \
	go build -tags=duckdb,duckdb_use_static_lib -mod vendor -ldflags="-s -w" \
	-o bin/embeddingsdb-server cmd/server/main.go
```

_Note: You don't have to copy `libduckdb_bundle.a` in to a local `work` folder but this way you don't have remember where it is or what happened to it the next time you clean up your `/usr/local/src` directory. The `work` directory is explicitly excluded from Git checkins in this repository._

## See also

* https://github.com/sfomuseum/go-embeddings
* https://github.com/sfomuseum/swift-mobileclip