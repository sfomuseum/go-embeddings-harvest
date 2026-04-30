## Databases

There are currently (4) supported database implemetations:

* [DuckDB](#duckdb) - manages vector embeddings using the [DuckDB](https://duckdb.org/) database and the [VSS](https://duckdb.org/docs/stable/core_extensions/vss) extension. This is the default implementation.
* [SQLite](#sqlite) - manages vector embeddings using the [SQLite](https://www.sqlite.org/) database and the the [sqlite-vec](https://github.com/asg017/sqlite-vec/tree/main) extension.
* [Bleve](#bleve) - manages vector embeddings using the [Bleve](https://github.com/blevesearch/bleve) database and the [faiss](https://github.com/blevesearch/faiss) library.
* [S3Vectors](#s3vectors) - manages vector embeddings using the Amazon Web Services [S3Vectors](https://aws.amazon.com/s3/features/vectors/) service.

Here's the "tl;dr":

The DuckDB implementation is generally faster than the SQLite but requires that all your data be stored in memory. That data is periodically exported to disk in order that it may be re-imported without indexing all the data from scratch but it takes a noticeable amount of time to import that data at start up time.

The SQLite implementation while has slower query times but stores (and reads) all its data from disk so it is fast to start.

The Bleve implementation is also fast, has a fast start-up time, doesn't require loading all the data in to memory, doesn't use an unmanageable amount of disk space but remains a non-trivial chore to set up because of the dependency on `libfaiss` (see details below). It's also unclear to me whether it is possible to create a single, bundled executable of the Bleve implementation because of the `libfaiss` depedency.

The S3Vectors implementation is fast and demonstrates good query times. It is, however, dependent on a commercial service (Amazon Web Services (AWS)) where everything (from storage to queries) is [metered](https://aws.amazon.com/s3/pricing/?nc=sn&loc=4). Depending on how your database access is configured this could lead to very large bills at the end of the month. If you have already made your peace with AWS then it can be a quick and easy way to get started with vector embeddings.

### duckdb://

Manage embeddings use the [DuckDB](https://duckdb.org/) database and the [VSS](https://duckdb.org/docs/stable/core_extensions/vss) extension.

```
duckdb://{PATH}?{QUERY_PARAMETERS}
```

Where `{PATH}` is an optional value mapped to the location of an existing DuckDB database. If present this database will be used to instantiate the database. Depending on the size of the database this can take a noticeable amount of time. It is also the location where the database will exported to if the `Server.Export` method is called.

Valid parameters are:

| Key | Value | Required | Notes |
| --- | --- | --- | --- |
| dimensions | int | no | The number of dimensions for the embeddings being stored. Default is 512. |
| max-distance | float | no | Update the default maximum distance when querying for similar embeddings. Default is 1.0. |
| max-results | int | no | Update the default number of records to return when querying	for similar embeddings.	Default	is 10. |

For example:

```
duckdb:///usr/local/data/embeddings
```

### sqlite://

Manage embeddings use the [SQLite](https://www.sqlite.org/) database and the [sqlite-vec](https://github.com/asg017/sqlite-vec/tree/main) extension.

```
sqlite://?{QUERY_PARAMETERS}
```

Valid parameters are:

| Key | Value | Required | Notes |
| --- | --- | --- | --- |
| dsn | string | yes | A registered `database/sql.Driver` DSN string. |
| dimensions | int | no | The number of dimensions for the embeddings being stored. Default is 512. |
| max-distance | float | no | Update the default maximum distance when querying for similar embeddings. Default is 1.0. |
| max-results | int | no | Update the default number of records to return when querying	for similar embeddings.	Default	is 10. |
| compression | string | no | The type of compression to use when storing embeddings. Options are: none, quantized, matroyshka. Default is "none". |

For example:

```
sqlite://?dsn=file:/usr/local/data/embeddings.db
```

_Note: As of this writing only the Go-language [CGO bindings](https://github.com/asg017/sqlite-vec-go-bindings?tab=readme-ov-file#cgo-bindings) are supported. Support for "pure Go" bindings will be added in future releases._

### bleve://

Manage embeddings use the [Bleve](https://blevesearch.com/) document store.

```
bleve://{PATH}?{QUERY_PARAMETERS}
```

If `{PATH}` is omitted then an in-memory database will be created.

Valid parameters are:

| Key | Value | Required | Notes |
| --- | --- | --- | --- |
| dimensions | int | no | The number of dimensions for the embeddings being stored. Default is 512. |
| max-distance | float | no | Update the default maximum distance when querying for similar embeddings. Default is 5.0. |
| max-results | int | no | Update the default number of records to return when querying	for similar embeddings.	Default	is 10. |

For example:

```
bleve:///usr/local/data/bleve-embeddings
```

#### Building (DuckDB)

Under the hood the Bleve implementation stores the static vector embeddings data in a separate DuckDB database. This is because the vector embeddings stored in Bleve itself are not returned as part of normal search queries and storing those data internally (to Bleve, outside of the search index) consumes an obscene amount of disk space. DuckDB simply uses less disk space.

What this means, practically, when building a Bleve-backed implementation of the tools in this package is you will need to do the `go mod tidy && go mod vendor` dance, described below, to pull in the DuckDB `.a` files. Everything else should be handled internally and not your concern.

#### Building (libfaiss)

This is a bit of a chore on a Mac. If you have already installed `libfaiss` from Homebrew (or whatever) you need to remove it and install the Bleve-specific fork:

```
$> git clone ssh://git@github.com/blevesearch/faiss.git
$> cd faiss

$> export LDFLAGS="-L/opt/homebrew/opt/llvm/lib" \
$> export CPPFLAGS="-I/opt/homebrew/opt/llvm/include" \
$> export CXX=/opt/homebrew/opt/llvm/bin/clang++ \
$> export CC=/opt/homebrew/opt/llvm/bin/clang \

$> cmake -B build \
  -DFAISS_ENABLE_GPU=OFF \
  -DFAISS_ENABLE_C_API=ON \
  -DBUILD_SHARED_LIBS=ON \
  -DFAISS_ENABLE_PYTHON=OFF .

$> make -C build
$> sudo make -C build install
$> sudo cp build/c_api/libfaiss_c.dylib /usr/local/lib
```

_Note that I had to use a completely different set of instructions to get `libfaiss` to compile on an Intel Mac. I don't know. For build instructions for Linux and Windows please consult the [Bleve documentation](https://github.com/blevesearch/bleve/blob/master/docs/vectors.md#setup-instructions)._

#### Building (Bleve)

If that weren't enough the current versioned Bleve release (2.5.7) is not current with changes in either the Bleve fork or `libfaiss` or [blevesearch/go-faiss](https://github.com/blevesearch/go-faiss) so, for the time being, the "easiest" thing is just to clone the most recent build of [blevesearch/bleve](https://github.com/blevesearch/bleve) locally and point to it from a [go.work](https://go.dev/doc/tutorial/workspaces) file. This is not ideal but it's less less-ideal than the alternatives.

```
$> cd /usr/local/src/
$> git clone https://github.com/blevesearch/bleve.git /usr/local/src/bleve
$> cd /usr/local/src/bleve
$> go mod tidy && go mod vendor
```

Now come back to _this_ repository and run:

```
$> go work init
```

Edit the `go.work` file to look like this (adjusting for wherever you are keeping your copy of the Bleve source code:

```
go 1.26.2

use (
    ./
    /usr/local/src/bleve
)    
```

Remember that you also need to include the `-tags vectors` and `-ldflags -r /usr/local/lib` when you build things. For example:

```
$> make cli TAGS=sqlite,bleve,vectors LDFLAGS='-s -w -r /usr/local/lib'
go build -tags=sqlite,bleve,vectors -mod readonly -ldflags="-s -w -r /usr/local/lib" -o bin/embeddingsdb-client cmd/client/main.go
...and so on
```

#### Other "known knowns"

I have observed that under some conditions importing large datasets (using the `parquet-import` tool for example) data corruption can occur. This problem _seems_ to be related to memory-mapping and the `go.etcd.io/bbolt` package but I am not certain. These problems seem to have been resolved on Apple Silicon Macs but I continue to experience them on older Intel-based Macs.

The Bleve source code specifies `bbolt` v1.4.0 even though the last release is 1.4.3 but even that was in 2025 and there have been lots of updates to the source code. I've tried both specifying v1.4.3 and using a `go.work` file to use the most recent code but database corruption and the occassional race condition still manifest on Intel-based Macs.

That said, I am not confident that I have even diagnosed the problem correctly.

### s3vectors://

Manage embeddings use the Amazon Web Services (AWS) [S3Vectors](https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-vectors.html) service. This database implementation relies on a commercial service is metered. Depending on how your database access is configured this could lead to very large bills at the end of the month. If you have already made your peace with AWS then it can be a quick and easy way to get started with vector embeddings.

```
s3vectors://{BUCKET_NAME}?{QUERY_PARAMETERS}
```

Where `{BUCKET_NAME}` is the name of the S3Vectors bucket where embeddings are stored. This will be created dynamically at runtime if it does not already exist.

Valid parameters are:

| Key | Value | Required | Notes |
| --- | --- | --- | --- |
| index | string | yes | The name of the S3Vectors index where embeddings are stored. This will be created dynamically at runtime if it does not already exist. |
| region | string | yes | The AWS region where your S3Vectors bucket is stored. |
| credentials | string | yes | A valid `aaronland/go-aws/v3/auth` credentials string. Details are discussed below.  |
| dimensions | int | no | The number of dimensions for the embeddings being stored. Default is 512. |
| max-distance | float | no | Update the default maximum distance when querying for similar embeddings. Default is 1.0. |
| max-results | int | no | Update the default number of records to return when querying	for similar embeddings.	Default	is 10. |
| refresh-tags | bool | no | A boolean flag to update denormalized database properties in to index-specific "tags". Details are discussed below. |

For example:

```
s3vectors://embeddings-bucket?index=embeddings-1024?region=us-east-1&credentials=iam:&dimensions=1024
```

#### AWS credentials

Under the hood this package uses the [aaronland/go-aws/v3/auth](https://github.com/aaronland/go-aws/tree/main/auth) package for deriving AWS credentials using string labels. Valid labels are:

| Label | Description |
| --- | --- |
| `anon:` | Empty or anonymous credentials. |
| `env:` | Read credentials from AWS-defined environment variables. |
| `iam:` | Assume AWS IAM credentials are in effect. |
| `iam:{REGION}:{ARN}` | Assume AWS IAM credentials are in effect after assuming the IAM Role defined by `{ARN}` (in `{REGION}`). |
| `sts:{ARN}` | Assume the role defined by `{ARN}` using STS credentials. |
| `{AWS_PROFILE_NAME}` | This this profile from the default AWS credentials location. |
| `{AWS_CREDENTIALS_PATH}:{AWS_PROFILE_NAME}` | This this profile from a user-defined AWS credentials location. |

#### Refreshing database properties "tags"

The nature of the S3Vectors service means there is no way to quickly derive properties about a "database", like the list of unique models or providers, without crawling the entire data set. To account for this these data are compiled as necessary and stored as index-level "tags" which are read at start-up time to prevent excessive (and potentially expensive) repeated crawling of the index. If you need or want to explicitly refresh those data (tags) you can include the `?refesh-tags=true` query parameter with your URI constructor.