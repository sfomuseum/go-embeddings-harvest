## Clients

There are currently (2) supported client implementations:

* [gRPC](#grpc) - interact with a `go-embeddings` database service via [gRPC](https://grpc.io/).
* [Database](#database) - interact with a `go-embeddings` database directly.

### grpc://

Create a gRPC-based client for managing embeddings-related operations. Clients are created using a URI-based syntax as follows:

```
grpc://{HOST}:{ADDRESS}?{QUERY_PARAMETERS}
```

Valid parameters are:

| Key | Value | Required | Notes |
| --- | --- | --- | --- |
| token-uri | string | no | A registered `gocloud.dev/runtimevar` URI used to stored a shared authentication to require with client requests. |
| tls-certificate | string | no | The path to a valid TLS certificate to use for encrypted connections. |
| tls-ca-certificate | string | no | The path to a custom TLS authority certificate to use for encrypted connections. |
| tls-insecure | bool | no | Skip TLS verification steps. Use with caution. |

For example:

```
grpc://localhost:8080?token-uri=constant%3A%2F%2F%3Fval%3Ds33kret
```

### database://

Create a client with a direct database connection for managing embeddings-related operations. Clients are created using a URI-based syntax as follows:

```
database://?{QUERY_PARAMETERS}
```

Valid parameters are:

| Key | Value | Required | Notes |
| --- | --- | --- | --- |
| database-uri | string | yes | A registered `sfomuseum/go-embeddingsdb/database.Database` URI for the underlying database implementation to use. |

For example:

```
database://?database-uri=duckdb:///usr/local/data/embeddings
```
