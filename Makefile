BINARY      := blessedbot
MODULE      := github.com/t0mer/blessed-by-the-bot
VERSION     ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
IMAGE       := techblog/blessed-by-the-bot

export CGO_ENABLED=0

.PHONY: all build build-all run dev test test-race lint fmt tidy web web-install docker clean

all: lint test build

build: ## build the binary into bin/ (uses whatever is already in the embed dir)
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

build-all: web build ## build the frontend, then the binary that embeds it

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

web-install: ## install frontend dependencies
	cd web && npm ci

web: ## build the frontend straight into internal/webui/dist
	cd web && npm ci && npm run build

dev: ## run the API and the Vite dev server together with hot reload
	@./scripts/dev.sh

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):dev .

clean:
	rm -rf bin
