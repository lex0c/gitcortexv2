BINARY  := gitcortex
MODULE  := github.com/lex0c/gitcortexv2
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
GOFLAGS := -trimpath $(LDFLAGS)

.PHONY: build test vet clean install check

build:
	go build $(GOFLAGS) -o $(BINARY) ./cmd/gitcortex/

test:
	go test ./... -count=1

vet:
	go vet ./...

clean:
	rm -f $(BINARY)

install:
	go install $(GOFLAGS) ./cmd/gitcortex/

check: vet test
