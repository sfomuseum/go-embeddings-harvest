GOMOD=$(shell test -f "go.work" && echo "readonly" || echo "vendor")
LDFLAGS=-s -w

TAGS=sqlite

vuln:
	govulncheck -show verbose ./...

# godoc over http is deprecated but
# go install golang.org/x/tools/cmd/godoc@latest

godoc:
	godoc -http=:6060

test:
	go test -tags $(TAGS) -v ./...

fix:
	go test -tags $(TAGS) ./...

cli:
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/embeddingsdb-client cmd/client/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/embeddingsdb-server cmd/server/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/embeddingsdb-inspector cmd/inspector/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-export cmd/parquet-export/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-import cmd/parquet-import/main.go

bleve:
	@make cli TAGS=sqlite,vectors,bleve LDFLAGS='-s -w -r /usr/local/lib'

wasmjs:
	GOOS=js GOARCH=wasm \
		go build -mod $(GOMOD) -ldflags="$(LDFLAGS)" -tags wasmjs \
		-o oembeddings/www/wasm/oembeddings_validate.wasm \
		cmd/oembeddings-validate-wasm/main.go

inspector:
	go run -tags=$(TAGS) -mod $(GOMOD) \
		cmd/inspector/main.go \
		-verbose \
		-client-uri 'grpc://localhost:8081' \
		-enable-uploads \
		-embeddings-client-uri 'mobileclip://?client-uri=grpc://localhost:8080' \
		-server-uri http://localhost:8082

server-bundle:
	CGO_ENABLED=1 CPPFLAGS="-DDUCKDB_STATIC_BUILD" CGO_LDFLAGS="-L./work -lduckdb_bundle -lc++" go build -tags=duckdb,duckdb_use_static_lib -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/embeddingsdb-server cmd/server/main.go

# https://developers.google.com/protocol-buffers/docs/reference/go-generated
# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

proto:
	protoc --go_out=. --go_opt=paths=source_relative --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative grpc/org_sfomuseum_embeddingsdb_service.proto

