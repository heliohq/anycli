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
# -count=1 disables go's test cache: the env override is read via
# os.Environ(), which the cache cannot track, so a cached run would
# silently replay a stale (e.g. credential-less skip) result.
e2e:
	@test -n "$(TOOL)" || (echo "usage: make e2e TOOL=<tool-name>" && exit 1)
	go test -tags e2e -count=1 -v -run 'TestE2E' ./internal/tools/$(shell echo $(TOOL) | tr -d '-')/...
