BINARY := any
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64
DIST := dist

.PHONY: build clean test dist all

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

test:
	go test ./...

clean:
	rm -rf $(BINARY) $(DIST)

dist: clean
	@mkdir -p $(DIST)
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		name=$(BINARY)_$(VERSION)_$${os}_$${arch}; \
		echo "building $${name}..."; \
		GOOS=$${os} GOARCH=$${arch} go build -ldflags "$(LDFLAGS)" -o $(DIST)/$${name}/$(BINARY) . ; \
		tar -czf $(DIST)/$${name}.tar.gz -C $(DIST)/$${name} $(BINARY); \
		rm -rf $(DIST)/$${name}; \
	done

all: test dist
