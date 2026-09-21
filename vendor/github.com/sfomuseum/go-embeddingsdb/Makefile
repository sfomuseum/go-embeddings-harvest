GOMOD=$(shell test -f "go.work" && echo "readonly" || echo "vendor")
LDFLAGS=-s -w

TAGS=
EMBEDDINGS_CLIENT=mobileclip://?client-uri=grpc://localhost:8080

vuln:
	govulncheck -show verbose ./...

# godoc over http is deprecated but
# go install golang.org/x/tools/cmd/godoc@latest

godoc:
	godoc -http=:6060

build:
	go build -tags duckdb ./...

test:
	go test -tags duckdb -ldflags="$(LDFLAGS) -r /usr/local/lib" -v ./...

fix:
	go fix -tags duckdb -ldflags="$(LDFLAGS) -r /usr/local/lib" ./...

cli:
	@make cli-server
	@make cli-client
	@make cli-inspector
	@make cli-parquet

cli-server:
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/embeddingsdb-server cmd/server/main.go

cli-client:
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/embeddingsdb-client cmd/client/main.go

cli-parquet:
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-merge cmd/parquet-merge/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-export cmd/parquet-export/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-import cmd/parquet-import/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-metadata cmd/parquet-metadata/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-gather-stats cmd/parquet-gather-stats/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-append-stats cmd/parquet-append-stats/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-emit cmd/parquet-emit/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-sign cmd/parquet-sign/main.go
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/parquet-verify cmd/parquet-verify/main.go

cli-inspector:
	go build -tags=$(TAGS) -mod $(GOMOD) -ldflags="$(LDFLAGS)" -o bin/embeddingsdb-inspector cmd/inspector/main.go

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
		-enable-search \
		-embeddings-client-uri "$(EMBEDDINGS_CLIENT)" \
		-server-uri http://localhost:8082

lambda-inspector:
	if test -f bootstrap; then rm -f bootstrap; fi
	if test -f embeddingsdb-inspector.zip; then rm -f embeddingsdb-inspector.zip; fi
	GOARCH=arm64 GOOS=linux go build -mod $(GOMOD) -ldflags="$(LDFLAGS)" -tags no_duckdb,lambda.norpc -o bootstrap cmd/inspector/main.go
	zip embeddingsdb-inspector.zip bootstrap
	rm -f bootstrap


server-bundle:
	CGO_ENABLED=1 CPPFLAGS="-DDUCKDB_STATIC_BUILD" CGO_LDFLAGS="-L./work -lduckdb_bundle -lc++" go build -tags=duckdb,duckdb_use_static_lib -mod $(GOMOD) -ldflags="$(LDFLAGS) -r /usr/local/lib" -o bin/embeddingsdb-server cmd/server/main.go

# https://developers.google.com/protocol-buffers/docs/reference/go-generated
# go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

proto:
	protoc --go_out=. --go_opt=paths=source_relative --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative grpc/org_sfomuseum_embeddingsdb_service.proto

