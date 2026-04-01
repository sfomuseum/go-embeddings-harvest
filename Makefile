CWD=$(shell pwd)

GOMOD=$(shell test -f "go.work" && echo "readonly" || echo "vendor")
LDFLAGS=-s -w

vuln:
	govulncheck -show verbose ./...

cli:
	go build -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/harvest-flickr-embeddings cmd/harvest-flickr-embeddings/main.go
	go build -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/harvest-sfomuseum-media-embeddings cmd/harvest-sfomuseum-media-embeddings/main.go
	go build -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/harvest-sfomuseum-instagram-embeddings cmd/harvest-sfomuseum-instagram-embeddings/main.go
