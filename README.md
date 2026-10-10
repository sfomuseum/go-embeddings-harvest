# go-embeddings-harvest

Go package for harvesting data from a variety of providers, deriving vector embeddings for those data and writing everything to Parquet files.

## Motivation

The broader aim is to try and establish what the "simplest and dumbest" amount of metadata is necessary for two or more cultural heritage institutions to share vector embedding data, of their respective collections or holdings, in order to perform cross-institutional similarity queries. Here's a picture to illustrate that idea:

```
+------------------------+      +-----------------------+      +---------------------+

| Raw Museum Open Data   | ---> | go-embeddings-harvest | ---> | Shared Parquet File |
| (CMA, MoMA, NGA, etc.) |      +-----------------------+      +---------------------+
+------------------------+                  |                             |
                                            v                             v
                                  +-------------------+         +--------------------+

                                  | Local Blob Cache  |         | go-embeddingsdb    |
                                  |     (Images)      |         | Database Server    |
                                  +-------------------+         +--------------------+
```

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

### harvest-embeddings

This tool produces a Parquet file containing rows, for a given source (a "harvester" described below), which map to the `Record` data structure described above. They have been designed to work in concert with tools like the [parquet-import](https://github.com/sfomuseum/go-embeddingsdb?tab=readme-ov-file#parquet-import) application which is designed to import these data files in a [sfomuseum/go-embeddingsdb](https://github.com/sfomuseum/go-embeddingsdb?tab=readme-ov-file#parquet-import) database server instance.

```
$> ./bin/harvest-embeddings -h
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
    	A registered sfomuseum/go-embeddings-harvest.Harvester URI. Valid options are: cma://, moma://, nga://, null://, sfomuseum://, si:// (default "null://")
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

For example, derive embeddings from the [Cleveland Museum of Art (CMA) open data release](https://github.com/ClevelandMuseumArt/openaccess) using the Google [https://huggingface.co/google/siglip2-base-patch16-naflex](https://huggingface.co/google/siglip2-base-patch16-naflex) model saving that data to a Parquet file called `cma-naflex.parquet`:

```
$> ./bin/harvest-embeddings \
	-harvester-uri cma:///usr/local/data/cma/openaccess/data.csv \
	-embeddings-client-uri 'siglip-client://?client-uri=http://localhost:5000' \	
	-cache-uri file:///usr/local/data/blobcache/ \
	-output work/cma-naflex.parquet	
```

The `-harvester-uri`, `-embeddings-client-uri` and `-cache-uri` flags are discussed in the [Harvester](#harvesters), [Embeddings clients](#embeddings-client) and [Caches](#caches) sections below.

#### Harvesters

Harvesters implement the `Harvester` interface to return records suitable for storing in a [sfomuseum/go-embeddingsdb](https://github.com/sfomuseum/go-embeddingsdb) database instance. That interface looks like this:

```
type Harvester interface {
	Iterate(context.Context, *IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error]
	Close() error
}
```

Harvesters are instantiated using the `harvest.NewHarvester(ctx, uri)` method where the details of the source data (used to create a list of iterable `*embeddingsdb.Record` records) are expected to be encoded in `uri`.

#### Cleveland Museum of Art 

Derive embeddings for object images in the [Cleveland Museum of Art (CMA) open data release](https://github.com/ClevelandMuseumArt/openaccess). The CMA harvester expects a URI in the form of:

```
cma://{PATH_TO_OPENACCESS_DATA.CSV}
```

For example:

```
cma:///usr/local/data/cma/openaccess/data.csv
```

#### Flickr

_Derive embeddings for images using the [Flickr API](https://www.flickr.com/services/api/). This harvester has been temporarily removed but will return shortly._

```
flickr://{PROVIDER_NAME}/{FLICKR_SPR_PATH}?client_uri={FLICKR_API_CLIENT_RUNTIMEVAR_URI}
```

For example:

```
flickr://flickr-commons-powerhouse?client-uri=file:///usr/local/secrets/flickr/client.txt
```

#### Museum of Modern Art

Derive embeddings for object images in the [Museum of Modern Art (MoMA) open data release](https://github.com/MuseumofModernArt/collection). The MoMA harvester expects a URI in the form of:

```
moma://{PATH_TO_COLLECTION_ARTWORKS.CSV}
```

For example:

```
moma:///usr/local/data/moma/collection/Artworks.csv
````

#### National Gallery of Art

Derive embeddings for object images in the [National Gallery of Art (NGA) open data release](https://github.com/NationalGalleryOfArt/opendata). The NGA harvester expects a URI in the form of:

```
nga://{PATH_TO_OPENDATA_OBJECTS.CSV}?images={PATH_TO_OPENDATA_IMAGES.CSV}
```

For example:

```
nga:///usr/local/data/nga/opendata/data/objects.csv?images=/usr/local/data/nga/opendata/data/published_images.csv
````

#### SFO Museum

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
| --- | --- | --- | --- |
| `parent-reader-uri` | string  | no | A registered [whosonfirst/go-reader.Reader](https://github.com/whosonfirst/go-reader/blob/main/README.md) URI used to read data for parent records. Default is "https://data.whosonfirst.org". |
| `iterator-uri` | string | no | A registered [whosonfirst/go-whosonfirst/v4/iterate.Iterator](https://github.com/whosonfirst/go-whosonfirst/blob/main/iterate/README.md) URI used to indicate how source data should be processed. Default "repo://". |
| `iterator-source` | string | yes | One or more URIs referencing source data to be harvested. | 

For example:

```
sfomuseum://sfomuseum-data-socialmedia-instagram?iterator-source=/usr/local/data/sfomuseum-data-socialmedia-instagram
```

#### Smithsonian

Derive embeddings for object images in the [Smithsonian (SI) OpenAccess data release](https://github.com/Smithsonian/OpenAccess). The SI harvester expects a URI in the form of:

```
si://?{QUERY_PARAMETERS}
```

Where valid query parameters are:

| Name | Value | Required | Notes |
| --- | --- | --- | --- |
| `bucket-uri` | string | No | This is the source of SI data to harvest [as described in `aaronland/go-smithsonian-openaccess` package](https://github.com/aaronland/go-smithsonian-openaccess#data-sources). If left empty then the harvester will harvest data from the Smithsonian's public (AWS) S3 bucket. |
| `unit` | string | Yes | One or more Smithsonian "unit" labels . | 

For example:

```
si://?unit=nmah&unit=nasm
````

#### Embeddings clients

Under the hood, harvesters use the [sfomuseum/go-embeddings](https://github.com/sfomuseum/go-embeddings) package to instantiate the "clients" that are used to generate	vector embeddings for a	source (image). Like harvesters these clients are instantiated using a URI-based syntax.

There are many ways to generate vector embeddings, each with their own tradeoffs and resource constraints. To account for these differences harvesters don't make any assumptions about how embeddings are created. That decision is left up to you which, in turn, will require some additional setup.

In the example, above, embeddings are being generated using [the `siglip-client://` embeddings client](https://github.com/sfomuseum/go-embeddings#client-server-siglip-client). This implementation also expects [a separate HTTP service listening on port 5000](https://github.com/sfomuseum/container-siglip#endpoints) to be present and responsible for generating embeddings. These details are not handled, in any kind of automated fashion, by code in this package.
 
It's not ideal but it's better than assuming we know what is best for you. For the complete list of embeddings client implementations please consult [the `sfomuseum/go-embeddings` documentation](https://github.com/sfomuseum/go-embeddings#implementations).

#### Caches

Image files that are referenced by harvester sources may be cached locally to speed up processing (for example, when generating embeddings using different models). This happens using the [sfomuseum/go-blobcache](https://github.com/sfomuseum/go-blobcache) package. This package allows images to be cached [locally or using a remote storage service](https://github.com/sfomuseum/go-blobcache#providers) (for example AWS S3).

Consult [the documentation for complete details](https://github.com/sfomuseum/go-blobcache#providers) but the easiest way to use it is to simply reference a local path on disk. For example:

```
file:///usr/local/data/blobcache	
```

## Implementing a custom harvester

The easiest way to get started implementing a custom harvester is to clone and modify [the "Null" harvester](harvester_null.go). This implements the `Harvester` interface but yields no records. To implement a custom harvester you might do something like this:

```
package custom

import (
	"context"
	"iter"

	"github.com/sfomuseum/go-embeddings-harvest"	
	"github.com/sfomuseum/go-embeddingsdb"
)

func init() {

	// This is important. This is what enables the following to work:
	// h, err := harvest.NewHarvester(ctx, "custom://")
	
	harvest.MustRegisterHarvester(context.Background(), "custom", NewCustomHarvester)
}

type CustomHarvester struct {
	harvest.Harvester
}

func NewCustomHarvester(ctx context.Context, uri string) (harvest.Harvester, error) {

	// Parse 'uri' here storing any relevant details in 'h' remembering
	// to update the type definition for `CustomHarvester` accordingly.
	
	h := &CustomHarvester{}
	return h, nil
}

func (h *CustomHarvester) Iterate(ctx context.Context, opts *harvest.IterateOptions) iter.Seq2[[]*embeddingsdb.Record, error] {

	return func(yield func([]*embeddingsdb.Record, error) bool) {

		// Harvest custom data here calling yield(records, error)
		// as necessary
		
		return
	}
}

func (h *CustomHarvester) Close() error {
	return nil
}
```

Once you have created your harvester you can either submit it [as a PR for inclusion with this package](https://github.com/sfomuseum/go-embeddings-harvest/issues) or use it privately in your code. If you are opting for the latter (private) approach you will also need to clone the `cmd/embeddings-harvest/main.go` code in to a new tool and import your custom package. Since the "guts" of the `embeddings-harvest` tool live in the [app/harvest](app/harvest) package this process should be as easy as this:

```
package main

import (
	"context"
	"log"

	_ "github.com/custom-org/harvester/custom"
	
	"github.com/sfomuseum/go-embeddings-harvest/app/harvest"
)

func main() {

	ctx := context.Background()
	err := harvest.Run(ctx)

	if err != nil {
		log.Fatalf("Failed to harvest embeddings, %v", err)
	}
}
```

## See also

* https://github.com/sfomuseum/go-embeddings
* https://github.com/sfomuseum/go-embeddingsdb