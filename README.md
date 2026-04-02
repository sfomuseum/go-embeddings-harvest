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
* Could Flickr-style "machine tags", with a convention-based set of known prefixes, be enough?

The goal here is the establish the _least amount of metadata_ necessary to accurately reflect provenance and to provide avenues for machine-readable metadata to be derived on a case-by-case basis.

So the purpose of this package is to provide tools to generate Parquet-encoded representations of those data for a variety of sources to help work through those questions.

_Eventually embeddings for text-based sources will be supported but that hasn't happened yet._

## Tools

The easiest way to get started is to run the handy `cli` Makefile target to build the available tools. For example:

```
$> make cli
go build -mod vendor -ldflags="-s -w" -o bin/harvest-flickr-embeddings cmd/harvest-flickr-embeddings/main.go
go build -mod vendor -ldflags="-s -w" -o bin/harvest-nga-embeddings cmd/harvest-nga-embeddings/main.go
go build -mod vendor -ldflags="-s -w" -o bin/harvest-sfomuseum-media-embeddings cmd/harvest-sfomuseum-media-embeddings/main.go
go build -mod vendor -ldflags="-s -w" -o bin/harvest-sfomuseum-instagram-embeddings cmd/harvest-sfomuseum-instagram-embeddings/main.go
```

Each of these tools produces a Parquet file containing rows which map to the `Record` data structure described above. They have been designed to work in concert with tools like the [parquet-import](https://github.com/sfomuseum/go-embeddingsdb?tab=readme-ov-file#parquet-import) application which is designed to import these data files in a [sfomuseum/go-embeddingsdb](https://github.com/sfomuseum/go-embeddingsdb?tab=readme-ov-file#parquet-import) database server instance.

For example to generate data using the Flickr API and then import in to an `embeddingsdb` database you might do something like this:	

```
$> cd /usr/local/src/go-embeddings-harvest
$> ./bin/harvest-flickr-embeddings \
	-flickr-client-uri file:///usr/local/flickr.txt \
	-param user_id=49487266@N07 \
	-param method=flickr.photosets.getPhotos \
	-param photoset_id=72157710813888403 \
	-provider flickr-49487266@N07 \
	-spr-path photoset.photo \
	-model s0,s1,s2 \
	-output flickr.parquet \

$> cd /usr/local/src/go-embeddingsdb
$> ./bin/parquet-import -verbose -client-uri grpc://localhost:8081 /usr/local/src/go-embeddings-harvest/flickr.parquet
```

### harvest-flickr-embeddings

Generate Parquet-encoded embeddings from a Flickr API "standard photo response".

```
$> ./bin/harvest-flickr-embeddings -h
Generate Parquet-encoded embeddings from a Flickr API "standard photo response".
Usage:
	./bin/harvest-flickr-embeddings [options]Valid options are:
  -embeddings-client-uri string
    	A registered sfomuseum/go-embeddings.Client URI. (default "mobileclip://?client-uri=grpc://localhost:8080")
  -flickr-client-uri string
    	A gocloud/runtimevar URI which dereferences in to a valid aaronland/go-flickr-api/client.Client URI.
  -model value
    	One or more models to use to generate embeddings. This may also be a comma-separated string containing a list of models.
  -output string
    	The path where Parquet-encoded data should be written. If "-" then data will be written to STDOUT. (default "-")
  -param value
    	Zero or more {KEY}={VALUE} parameters to query the Flickr API with.
  -provider string
    	The name of the provider to assign to each embeddings record. (default "flickr")
  -spr-path string
    	The path to the list of photos in the Flickr API response. Paths should be described using tidwall/gjson "dot" notation.
  -verbose
    	Enable verbose (debug) logging.
```

For example, to derive embeddings from the San Diego Air and Space Museum's [California's Aviation Heritage](https://flickr.com/photos/sdasmarchives/albums/72157710813888403/) photoset:

```
./bin/harvest-flickr-embeddings \
	-flickr-client-uri file:///usr/local/flickr.txt \
	-param user_id=49487266@N07 \
	-param method=flickr.photosets.getPhotos \
	-param photoset_id=72157710813888403 \
	-provider flickr-49487266@N07 \
	-spr-path photoset.photo \
	-model s0,s1,s2 \
	-output flickr.parquet \
	-verbose	
```

#### Flickr Standard Photos Response (`-spr-path`)

The [Standard Photos Response, APIs for a civilized age](https://code.flickr.net/2008/08/19/standard-photos-response-apis-for-civilized-age/) blog post from Flickr describes the standard photos response (SPR) this way:

> The standard photos response is a data structure that we use when we want to return a list of photos. Most prominently the ever popular swiss-army-API flickr.photos.search() uses it, but also methods like flickr.favorites.getList() or flickr.groups.pools.getPhotos().

The `harvest-flickr-embeddings` tool is designed to derive embeddings for any Flickr API method that returns an SPR-encoded list of photos. You will need to consult the [Flickr API documentation](https://www.flickr.com/api) to determine the (JSON) query path where photos are stored (it varies from API method to API method).

For example to retrieve photos from the [Airports – SFO](https://flickr.com/groups/airports-sfo/) group with permissive licensing (CreativeCommons or "no known copyright") you might do something like this:

```
$> ./bin/harvest-flickr-embeddings \
	-param group_id=95693046@N00 \
	-param method=flickr.photos.search \
	-param license=1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16
	-spr-path photos.photo \
	-flickr-client-uri file:///usr/local/flickr.txt \
	-verbose \
	-output flickr-sfo.parquet \
	-model s0,s1,s2
```

#### Flickr API client credentials URIs (`-flickr-client-uri`)

Under the hood this tool uses the [aaronland/go-flickr-api](https://github.com/aaronland/go-flickr-api) package to communicate with the Flickr API. The nature of the Flickr API means that you need to provide a long credentials URI like this:

```
oauth1://?consumer_key={KEY}&consumer_secret={SECRET}&oauth_token={TOKEN}&oauth_token_secret={SECRET}
```

_This_ package uses the [gocloud.dev/runtimevar](https://gocloud.dev/howto/runtimevar/) package to read those long-twisty URIs from a variety of sources. For the purposes of getting start the easiest thing is to put your `go-flickr-api` credentials URI in a local file and then refer to it as `file://path/to/file-containing-credentials`.

#### See also

* https://www.flickr.com/services/api/
* github.com/aaronland/go-flickr-api

### harvest-nga-embeddings

Generate Parquet-encoded embeddings from the National Gallery of Art (NGA) open data release.

```
$> ./bin/harvest-nga-embeddings -h
Generate Parquet-encoded embeddings from the National Gallery of Art (NGA) open data release.
Usage:
	./bin/harvest-nga-embeddings [options]Valid options are:
  -embeddings-client-uri string
    	A registered sfomuseum/go-embeddingsdb/client.Client URI. (default "mobileclip://?client-uri=grpc://localhost:8080")
  -model value
    	One or more models to derive embeddings for. This may also be a comma-separated list.
  -output string
    	The path where Parquet-encoded data should be written. If "-" then data will be written to STDOUT. (default "-")
  -published-images string
    	The path to the 'published_images.csv' file contained in the NationalGalleryOfArt/opendata GitHub repository.
  -verbose
    	Enable verbose (debug) logging.
```

For example:

```
$> ./bin/harvest-nga-embeddings \
	-output nga.parquet \
	-published-images /usr/local/src/opendata/data/published_images.csv \
	-model s0,s1,s2
```

#### See also

* https://github.com/NationalGalleryOfArt/opendata

### harvest-sfomuseum-instagram-embeddings

Generate Parquet-encoded embeddings from SFO Museum sfomuseum-data-socialmedia-instagram data repositories (aka "Instragram photos").

```
$> bin/harvest-sfomuseum-instagram-embeddings  -h
Generate Parquet-encoded embeddings from SFO Museum sfomuseum-data-socialmedia-instagram data repositories (aka "Instragram photos").
Usage:
	bin/harvest-sfomuseum-instagram-embeddings [options]Valid options are:
  -embeddings-client-uri string
    	A registered sfomuseum/go-embeddingsdb/client.Client URI. (default "mobileclip://?client-uri=grpc://localhost:8080")
  -iterator-source string
    	The source for the go-whosonfirst-iterate/v3.Iterator instance to process. (default "/usr/local/data/sfomuseum-data-socialmedia-instagram")
  -iterator-uri string
    	A registered go-whosonfirst-iterate/v3.Iterator URI. (default "repo://?exclude=properties.edtf:deprecated=.*")
  -model value
    	One or more models to derive embeddings for. This may also be a comma-separated list.
  -output string
    	The path where Parquet-encoded data should be written. If "-" then data will be written to STDOUT. (default "-")
  -verbose
    	Enable verbose (debug) logging.
```

#### See also

* github.com/sfomuseum-data/sfomuseum-data-socialmedia-instagram/

### harvest-sfomuseum-media-embeddings

Generate Parquet-encoded embeddings from SFO Museum sfomuseum-data-media* data repositorie.

```
$> bin/harvest-sfomuseum-media-embeddings -h
Generate Parquet-encoded embeddings from SFO Museum sfomuseum-data-media* data repositorie.
Usage:
	bin/harvest-sfomuseum-media-embeddings [options]Valid options are:
  -embeddings-client-uri string
    	A registered sfomuseum/go-embeddingsdb/client.Client URI. (default "mobileclip://?client-uri=grpc://localhost:8080")
  -iterator-source string
    	The source for the go-whosonfirst-iterate/v3.Iterator instance to process. (default "/usr/local/data/sfomuseum-data-media-collection")
  -iterator-uri string
    	A registered go-whosonfirst-iterate/v3.Iterator URI. (default "repo://?exclude=properties.edtf:deprecated=.*")
  -model value
    	One or more models to derive embeddings for. This may also be a comma-separated list.
  -output string
    	The path where Parquet-encoded data should be written. If "-" then data will be written to STDOUT. (default "-")
  -provider string
    	The name of the provider to assign to each embeddings record. (default "sfomuseum-data-media-collection")
  -verbose
    	Enable verbose (debug) logging.
```

#### See also

* github.com/sfomuseum-data/sfomuseum-data-media
* github.com/sfomuseum-data/sfomuseum-data-media-collection

## See also

* https://github.com/sfomuseum/go-embeddings
* https://github.com/sfomuseum/go-embeddingsdb