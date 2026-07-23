.PHONY: build vet test lint all build-harness e2e

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

# Run one tool's e2e tests: make e2e TOOL=attio
# Credentials come from HELIO_E2E_API_KEY + HELIO_E2E_API_BASE (gateway) or
# ANYCLI_E2E_CRED_* (local override) — see docs/e2e.md.
e2e:
	@test -n "$(TOOL)" || (echo "usage: make e2e TOOL=<tool-name>" && exit 1)
	go test -tags e2e -v -run 'TestE2E' ./internal/tools/$(shell echo $(TOOL) | tr -d '-')/...
