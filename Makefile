.PHONY: build vet test lint all build-harness

# AnyCLI is an embeddable library (design 002): there is no main package and no
# standalone binary to produce. These targets compile and check the packages.

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
