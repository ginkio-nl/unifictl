BINARY  := unifictl
PKG     := ./cmd/unifictl
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build install test vet fmt check clean help

all: check build ## Run checks and build

build: ## Build the binary into ./unifictl
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) $(PKG)

install: ## Install into $GOBIN (or $GOPATH/bin)
	go install -ldflags "$(LDFLAGS)" $(PKG)

test: ## Run tests with the race detector
	go test -race ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Format all Go files
	gofmt -w .

check: vet test ## Verify formatting, vet, and test
	@test -z "$$(gofmt -l .)" || { echo "Unformatted files (run 'make fmt'):"; gofmt -l .; exit 1; }

clean: ## Remove build output
	rm -f $(BINARY)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-10s %s\n", $$1, $$2}'
