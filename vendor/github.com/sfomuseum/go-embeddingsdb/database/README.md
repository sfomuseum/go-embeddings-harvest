## Databases

There are currently (4) supported database implemetations:

* [DuckDB](#duckdb) - manages vector embeddings using the [DuckDB](https://duckdb.org/) database and the [VSS](https://duckdb.org/docs/stable/core_extensions/vss) extension.
* [SQLite](#sqlite) - manages vector embeddings using the [SQLite](https://www.sqlite.org/) database and the the [sqlite-vec](https://pkg.go.dev/modernc.org/sqlite/vec) extension.
* [Bleve](#bleve) - manages vector embeddings using the [Bleve](https://github.com/blevesearch/bleve) database and the [faiss](https://github.com/blevesearch/faiss) library.
* [S3Vectors](#s3vectors) - manages vector embeddings using the Amazon Web Services [S3Vectors](https://aws.amazon.com/s3/features/vectors/) service.

Here's the "tl;dr":

The SQLite implementation while has slower query times than DuckDB but stores (and reads) all its data from disk so it is fast to start. It is enabled by default.

The DuckDB implementation is generally faster than the SQLite but requires that all your data be stored in memory. That data is periodically exported to disk in order that it may be re-imported without indexing all the data from scratch but it takes a noticeable amount of time to import that data at start up time. It is enabled with the `duckdb` build tag.

The Bleve implementation is also fast, has a fast start-up time, doesn't require loading all the data in to memory, doesn't use an unmanageable amount of disk space but remains a non-trivial chore to set up because of the dependency on `libfaiss` (see details in [database/README.md](database/README.md#bleve)) which is "finnicky" at best. It's also unclear to me whether it is possible to create a single, bundled executable of the Bleve implementation because of the `libfaiss` depedency. It is enabled with the `bleve` and `vector` build tags.

The S3Vectors implementation is fast and demonstrates good query times. It is, however, dependent on a commercial service (Amazon Web Services (AWS)) where everything (from storage to queries) is [metered](https://aws.amazon.com/s3/pricing/?nc=sn&loc=4). Depending on how your database access is configured this could lead to very large bills at the end of the month. If you have already made your peace with AWS then it can be a quick and easy way to get started with vector embeddings. It is enabled by default.

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

Manage embeddings use the [SQLite](https://www.sqlite.org/) database and the [sqlite-vec](https://pkg.go.dev/modernc.org/sqlite/vec) extension.

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

### bleve://

Manage embeddings use the [Bleve](https://blevesearch.com/) document store. **Support for Bleve should be considered experimental at best.** As of this writing, I can't get it to build pending changes to `libfaiss`, the `libfaiss` bindings, Bleve's fork of `libfaiss` or all (or some) of the above. It has started to feel like a "whack-a-mole" exercise and it's not clear that it worth the effort.

```
bleve://{PATH}?{QUERY_PARAMETERS}
```

If `{PATH}` is omitted then an in-memory database will be created.

Valid parameters are:

| Key | Value | Required | Notes |
| --- | --- | --- | --- |
| dimensions | int | no | The number of dimensions for the embeddings being stored. Default is 512. |
| similarity-metric | string | no | The similarity metric used when comparing embeddings. Consult https://github.com/blevesearch/bleve/blob/master/docs/vectors.md for details. Note: This can not be changed after a Bleve index is created. Default is "l2_norm". |
| optimize-for | string | no | The vector index optimization strategy to use. Consult https://github.com/blevesearch/bleve/blob/master/docs/vectors.md for details. Default is "latency". |  
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
```

Note: The `blevesearch/faiss` checkpoint is relevant and specific to the version of `blevesearch/bleve` being used. For details consult: https://github.com/blevesearch/bleve/blob/master/docs/vectors.md#pre-requisites

Now issue the following commands:

```
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

As of the "v2.6.0" release of `blevesearch/bleve` everything _should_ work. Per the documentation you can [sanity check](https://github.com/blevesearch/bleve/blob/master/docs/vectors.md#sanity-check) things as follows:

```
$> cd /usr/local/src/bleve
$> go test -ldflags "-r /usr/local/lib" ./... -tags=vectors
```

Assuming that all the tests pass you can build the tools in _this_ package. Remember that you also need to include the `-tags vectors` and `-ldflags -r /usr/local/lib` when you build things. For example:

```
$> make cli TAGS=bleve,vectors LDFLAGS='-s -w -r /usr/local/lib'
go build -tags=bleve,vectors -mod readonly -ldflags="-s -w -r /usr/local/lib" -o bin/embeddingsdb-client cmd/client/main.go
...and so on
```

#### Other "known knowns"

I have observed that under some conditions importing large datasets (using the `parquet-import` tool for example) data corruption can occur. This problem _seems_ to be related to memory-mapping and the `go.etcd.io/bbolt` package but I am not certain. These problems seem to have been resolved on Apple Silicon Macs but I continue to experience them on older Intel-based Macs.

### s3vectors://

Manage embeddings use the Amazon Web Services (AWS) [S3Vectors](https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-vectors.html) service. It also uses the AWS [DynamoDB](https://docs.aws.amazon.com/dynamodb/) service to store metadata properties to enable functionality not otherwise available by the `S3Vectors` service.

This database implementation relies on a commercial service that is metered. Depending on how your database access is configured this could lead to [very large bills](https://murraycole.com/posts/aws-s3-vectors-pricing-deep-dive) at the end of the month. If you have already made your peace with AWS then it can be a quick and easy way to get started with vector embeddings.

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
| dynamodb-table | string | no | Use a custom DynamoDB table name for storing and querying record data. Default is "s3vectors". | 
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

### IAM policies

The following are the _minimal_ IAM policies you will need to have to use an S3Vectors-backed database. The following policies work are designed to work with a minimalist Lambs function but these should be adjusted as needed to the specifics of your situation.

#### S3Vectors

```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "DiscoverBuckets",
            "Effect": "Allow",
            "Action": [
                "s3vectors:ListVectorBuckets"
            ],
            "Resource": "*"
        },
        {
            "Sid": "ReadAndQueryAllS3VectorIndices",
            "Effect": "Allow",
            "Action": [
                "s3vectors:GetIndex",
                "s3vectors:GetVectors",
                "s3vectors:QueryVectors",
                "s3vectors:ListVectors"
            ],
            "Resource": "arn:aws:s3vectors:{AWS_REGION}:{AWS_ACCOUNT_ID}:bucket/{BUCKET_NAME}/index/*"
        },
        {
            "Sid": "ManageAllS3VectorIndexTags",
            "Effect": "Allow",
            "Action": [
                "s3vectors:ListTagsForResource",
                "s3vectors:TagResource",
                "s3vectors:UntagResource"
            ],
            "Resource": "arn:aws:s3vectors:{AWS_REGION}:{AWS_ACCOUNT_ID}:bucket/{BUCKET_NAME}/index/*"
        },
        {
            "Sid": "ListIndicesInBucket",
            "Effect": "Allow",
            "Action": [
                "s3vectors:ListIndexes",
                "s3vectors:ListIndexes",
                "s3vectors:GetVectorBucket"
            ],
            "Resource": "arn:aws:s3vectors:{AWS_REGION}:{AWS_ACCOUNT_ID}:bucket/{BUCKET_NAME}"
        }
    ]
}
```

#### DynamoDB

Note the use of the `s3vectors` and `s3vectors_metadata` table names in the example below. These are the default values. If you reassign the value of the `s3vectors` table with the `?dynamodb-table={YOUR_TABLE}` parameter, described above, you will need to update this example to replace `s3vectors` and `s3vectors_metadata` with `{YOUR_TABLE}` and `{YOUR_TABLE}_metadata` respecitively.

```
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "DynamoDBTableCreateDescribeAndList",
            "Effect": "Allow",
            "Action": [
                "dynamodb:CreateTable",
                "dynamodb:DescribeTable",
                "dynamodb:ListTables"
            ],
            "Resource": [
                "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors",
                "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors/*",
                "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors_metadata",
                "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors_metadata/*",		
                "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/*"
            ]
        },
        {
            "Sid": "DynamoDBPutDelete",
            "Effect": "Allow",
            "Action": [
                "dynamodb:PutItem",
                "dynamodb:DeleteItem"
            ],
            "Resource": [
	        "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors",
	        "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors_metadata"
	    ]
        },
        {
            "Sid": "DynamoDBQueryOnTableAndGSI",
            "Effect": "Allow",
            "Action": [
                "dynamodb:Query"
            ],
            "Resource": [
                "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors",
  		"arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors_metadata",		
                "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors/index/by_provider_model",
                "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors/index/by_model_provider",
                "arn:aws:dynamodb:{AWS_REGION}:{AWS_ACCOUNT_ID}:table/s3vectors_metadata/index/GSI1"		
            ]
        }
    ]
}
```