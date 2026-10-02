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

Likewise, there are not conventions for what should be included in the `Attributes` property which is currently defined as a freeform key-value lookup. The goal is the establish the _least amount of metadata_ necessary to accurately reflect provenance and to provide avenues for machine-readable metadata to be derived on a case-by-case basis.

The current state of this work is reflected in the [OEmbeddings - What is the least amount of metadata necessary for shared vector embeddings?](https://millsfield.sfomuseum.org/blog/2026/04/15/oembeddings/) blog post. Here are the proposed set of attributes (dubbed "OEmbeddings") as implemented by this code:

<table class="table">
<thead>
<tr>
<th>Name</th>
<th>Type</th>
<th>Required</th>
<th>Notes</th>
</tr>
</thead>

<tbody>
<tr>
<td><strong>type</strong></td>
<td>string</td>
<td>yes</td>
<td>Either &ldquo;image&rdquo; or &ldquo;text&rdquo;.</td>
</tr>

<tr>
<td><strong>preview</strong></td>
<td>string</td>
<td>yes</td>
<td>The preview content for the vector embeddings. If <code>type</code> is &ldquo;text&rdquo; then this is expected to be a string. If <code>type</code> is &ldquo;image&rdquo; this is expected to be a string confirming to the JSON Schema &ldquo;uri&rdquo; type</td>
</tr>

<tr>
<td><strong>depiction_url</strong></td>
<td>uri</td>
<td>no</td>
<td>A web page (or resource) for the depiction used to create the vector embeddings.</td>
</tr>

<tr>
<td><strong>subject_url</strong></td>
<td>uri</td>
<td>yes</td>
<td>A web page (or resource) for the subject of the depiction used to create the vector embeddings.</td>
</tr>

<tr>
<td><strong>subject_title</strong></td>
<td>string</td>
<td>yes</td>
<td>The title of the subject of the depiction. This may be an empty string.</td>
</tr>

<tr>
<td><strong>subject_creditline</strong></td>
<td>string</td>
<td>yes</td>
<td>The creditline or attribution for the subject of the depiction. This may be an empty string.</td>
</tr>

<tr>
<td><strong>provider_name</strong></td>
<td>string</td>
<td>yes</td>
<td>The name of the provider (holder) of the subject being depicted.</td>
</tr>

<tr>
<td><strong>provider_url</strong></td>
<td>uri</td>
<td>yes</td>
<td>The primary web page for the provider (holder) of the subject being depicted.</td>
</tr>
</tbody>
</table>

For technical details and code implementations please consulting [the `oembeddings` documentation in `sfomuseum/go-embeddingsdb` package](https://github.com/sfomuseum/go-embeddingsdb/tree/main/oembeddings).

## Tools

The easiest way to get started is to run the handy `cli` Makefile target to build the available tools. For example:

```
$> make cli
go build -mod vendor -ldflags="-s -w" -o bin/harvest-embeddings cmd/harvest-embeddings/main.go
$> make cli
```

### havest-embeddings

This tool produces a Parquet file containing rows, for a given source (a "harvester" described below), which map to the `Record` data structure described above. They have been designed to work in concert with tools like the [parquet-import](https://github.com/sfomuseum/go-embeddingsdb?tab=readme-ov-file#parquet-import) application which is designed to import these data files in a [sfomuseum/go-embeddingsdb](https://github.com/sfomuseum/go-embeddingsdb?tab=readme-ov-file#parquet-import) database server instance.

```
> ./bin/harvest-embeddings -h
Generate Parquet file containing rows, for a given source (a "harvester"), which map to the `Record` data structure.
Usage:
	./bin/harvest-embeddings [options]Valid options are:
  -cache-check-lastmod
    	A boolean value to indicate whether the last modified date of an object to harvest should be compared against the local cache.
  -cache-uri string
    	A register gocloud.dev/blob.Bucket URI to use for caching images. If null:// then no images will be cached. (default "null://")
  -embeddings-client-uri string
    	A registered sfomuseum/go-embeddingsdb/client.Client URI. (default "mobileclip://?client-uri=grpc://localhost:8080")
  -harvester-uri string
    	A registered sfomuseum/go-embessings-harvest.Harvester URI. Valid options are: cma://, moma://, nga://, null://, sfomuseum://, si:// (default "null://")
  -model value
    	One or more models to derive embeddings for. This may also be a comma-separated list.
  -output string
    	The path where Parquet-encoded data should be written. If "-" then data will be written to STDOUT. (default "/dev/null")
  -precache
    	Fetch images from source and store in (blob) cache without generating embeddings. If true this flag will reassign -output to /dev/null.
  -verbose
    	Enable verbose (debug) logging.
  -workers int
    	The number of workers to use to fetch images (and derive embeddings) concurrently (default 5)
```	

For example, derive embeddings from the [Cleveland Museum of Art (CMA) open data release](https://github.com/ClevelandMuseumArt/openaccess) using the Google [FOO](#) model saving that data to a Parquet file called `cma-naflex.parquet`:

```
$> ./bin/harvest-embeddings \
	-harvester-uri cma:///usr/local/data/cma/openaccess/data.csv \
	-embeddings-client-uri 'siglip-client://?client-uri=http://localhost:5000' \	
	-cache-uri file:///usr/local/data/blobcache/
	-output work/cma-naflex.parquet	
```

The `-harvester-uri`, `-embeddings-client-uri` and `-cache-uri` flags are discussed in the [Harvester](#), [Embedding clients](#) and [Caches](#) sections below.

#### Harvesters

Harvesters implement the `Harvester` interface to return records suitable for storing in a [sfomuseum/go-embeddingsdb](#) database instance. That interface looks like this:

```
type Harvester interface {
	Iterate(context.Context, *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error]
	Close() error
}
```

Harvesters are instantiated using the `harvest.NewHarvester(ctx, uri)` method where the details of the source data (used to create a list of iterable `*embeddingsdb.Record` records) are expected to be encoded in `uri`.

#### cma://

Derive embeddings for object images in the [Cleveland Museum of Art (CMA) open data release](https://github.com/ClevelandMuseumArt/openaccess). The CMA harvester expects a URI in the form of:

```
cma://{PATH_TO_OPENACCESS_DATA.CSV}
```

For example:

```
cma:///usr/local/data/cma/openaccess/data.csv
```

#### flickr://

Derive embeddings for images using the [Flickr API](https://www.flickr.com/services/api/). This harvester has been temporarily removed but will return shortly.

#### moma://

Derive embeddings for object images in the [Museum of Modern Art (MoMA) open data release](https://github.com/MuseumofModernArt/collection). The MoMA harvester expects a URI in the form of:

```
moma://{PATH_TO_COLLECTION_ARTWORKS.CSV}
```

For example:

```
moma:///usr/local/data/moma/collection/Artworks.csv
````

#### nga://

Derive embeddings for object images in the [National Gallery of Art (NGA) open data release](https://github.com/NationalGalleryOfArt/opendata). The NGA harvester expects a URI in the form of:

```
nga://{PATH_TO_OPENDATA_OBJECTS.CSV}?images={PATH_TO_OPENDATA_IMAGES.CSV}
```

For example:

```
nga:///usr/local/data/nga/opendata/data/objects.csv?images=/usr/local/data/nga/opendata/data/published_images.csv
````

#### sfomuseum://

Derive embeddings for object images in the [SFO Museum (SFOM) opend data release](https://github.com/sfomuseum-data). The SFOM harvester expects a URI in the form of:

```
sfomuseum://{PROVIDER}?{QUERY_PARAMETERS}
```

Where valid providers are:

* `sfomuseum-data-media-collection` - Harvest data from the [sfomuseum-data/sfomuseum-data-media-collection](https://github.com/sfomuseum-data/sfomuseum-data-media-collection) repository containing object images from the SFO Museum Aviation collection.
* `sfomuseum-data-media` - Harvest data from the [sfomuseum-data/sfomuseum-data-media](https://github.com/sfomuseum-data/sfomuseum-data-media) repository containing installation images from SFO Museum exhibitions.
* `sfomuseum-data-socialmedia-instagram` - Harvest data from the [sfomuseum-data/sfomuseum-data-socialmedia-instagram](https://github.com/sfomuseum-data/sfomuseum-data-socialmedia-instagram) repository containing images from the SFO Museum Instagram account.

And valid query parameters are:

| Name | Value | Required | Notes |
| `parent-reader-uri` | string  | no | A registered [whosonfirst/go-reader.Reader](https://github.com/whosonfirst/go-reader/blob/main/README.md) URI used to read data for parent records. Default is "https://data.whosonfirst.org". |
| `iterator-uri` | string | no | A registered [whosonfirst/go-whosonfirst/v4/iterate.Iterator](https://github.com/whosonfirst/go-whosonfirst/blob/main/iterate/README.md) URI used to indicate how source data should be processed. Default "repo://". |
| `iterator-source` | string | yes | One or more URIs referencing source data to be harvested. | 

For example:

```
sfomuseum://sfomuseum-data-socialmedia-instagram?iterator-source=/usr/local/data/sfomuseum-data-socialmedia-instagram
```

#### si://

Derive embeddings for object images in the [Smithsonian (SI) OpenAccess data release](https://github.com/Smithsonian/OpenAccess). The SI harvester expects a URI in the form of:

```
si://?{QUERY_PARAMETERS}
```

Where valid query parameters are:

| Name | Value | Required | Notes |
| `bucket-uri` | string | No | This is the source of SI data to harvest [as described in `aaronland/go-smithsonian-openaccess` package](https://github.com/aaronland/go-smithsonian-openaccess#data-sources). If left empty then the harvester will harvest data from the Smithsonian's public (AWS) S3 bucket. |
| `unit` | string | Yes | One or more Smithsonian "unit" labels . | 

For example:

```
si://?unit=nmah&unit=nasm
````

#### Embedding clients

#### Caches

## See also

* https://github.com/sfomuseum/go-embeddings
* https://github.com/sfomuseum/go-embeddingsdb