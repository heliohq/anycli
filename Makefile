.PHONY: build vet test lint all build-harness

# AnyCLI is an embeddable library (design 002). The library itself produces no
# binary; the only main package is the standalone dev harness under cmd/anycli
# (see build-harness).

build:
	go build ./...

vet:
	go vet ./...

test:
	go test ./...

lint: vet

all: build vet test

build-harness: ## Build the standalone dev harness binary
	go build -o bin/anycli ./cmd/anycli
