GOMOD=$(shell test -f "go.work" && echo "readonly" || echo "vendor")
LDFLAGS=-s -w
CWD=$(shell pwd)

vulnup:
	go install golang.org/x/vuln/cmd/govulncheck@latest

vuln:
	govulncheck -show verbose ./...
