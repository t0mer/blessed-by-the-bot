BINARY      := blessedbot
MODULE      := github.com/t0mer/blessed-by-the-bot
VERSION     ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
IMAGE       := techblog/blessed-by-the-bot

export CGO_ENABLED=0

.PHONY: all build run test test-race lint fmt tidy web docker clean

all: lint test build

build: ## build the binary into bin/
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

run: ## run the server in dev mode
	go run ./cmd/$(BINARY) --dev

test:
	go test ./...

test-race:
	CGO_ENABLED=1 go test -race ./...

lint:
	go vet ./...
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, ran go vet only"

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

web: ## build the frontend into internal/webui/dist (no-op until phase 7)
	@if [ -f web/package.json ]; then cd web && npm ci && npm run build; \
	else echo "web/ not present yet; skipping"; fi

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):dev .

clean:
	rm -rf bin
