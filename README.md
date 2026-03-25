# go-embeddings-harvest

Go package for harvesting data from a variety of providers, deriving vector embeddings for those data and writing everything to Parquet files.

## Motivation

## Tools

```
$> make cli
go build -mod vendor -ldflags="-s -w" -o bin/harvest-flickr-embeddings cmd/harvest-flickr-embeddings/main.go
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

## See also

* https://github.com/sfomuseum/go-embeddings
* https://github.com/sfomuseum/go-embeddingsdb