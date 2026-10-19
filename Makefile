CWD=$(shell pwd)

GOMOD=$(shell test -f "go.work" && echo "readonly" || echo "vendor")
LDFLAGS=-s -w

vulnup:
	go install golang.org/x/vuln/cmd/govulncheck@latest

vuln:
	govulncheck -show verbose ./...

cli:
	go build -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/harvest-embeddings cmd/harvest-embeddings/main.go
	go build -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/harvest-flickr-commons-embeddings cmd/harvest-flickr-commons-embeddings/main.go
