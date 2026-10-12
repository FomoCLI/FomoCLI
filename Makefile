BINARY  := fomocli
PKG     := github.com/FomoCLI/FomoCLI
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(PKG)/internal/cli.Version=$(VERSION) \
	-X $(PKG)/internal/cli.Commit=$(COMMIT) \
	-X $(PKG)/internal/cli.Date=$(DATE)

.PHONY: build install test cover lint fmt demo clean snapshot

build: ## Build ./fomo
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/fomocli

install: ## Install into $GOBIN
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/fomocli

test: ## Run tests with the race detector
	go test -race ./...

cover: ## Coverage report
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

lint: ## Vet and format check
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed:"; gofmt -l .; exit 1)

fmt: ## Format code
	gofmt -w .

demo: build ## Try it with built-in sample data
	./$(BINARY) --demo lb

snapshot: ## Local multi-platform release build (needs goreleaser)
	goreleaser release --snapshot --clean

clean:
	rm -rf $(BINARY) dist coverage.out
