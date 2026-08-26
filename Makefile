BINARY      := blessedbot
MODULE      := github.com/t0mer/blessed-by-the-bot
VERSION     ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
IMAGE       := techblog/blessed-by-the-bot

export CGO_ENABLED=0

.PHONY: all build build-go run dev test test-go test-web test-race lint fmt tidy web web-install docker clean

all: lint test build

# The frontend first, then the binary that embeds it. A fresh clone must get a
# working UI from `make build` alone — `build-go` is the fast inner loop for
# when only Go code changed.
build: web build-go ## build the frontend and the binary that embeds it

build-go: ## build only the binary, reusing whatever is in the embed dir
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

run: ## run the server in dev mode
	go run ./cmd/$(BINARY) --dev

test: test-go test-web ## run the Go and frontend suites

test-go:
	go test ./...

test-web: ## type-check and test the frontend
	cd web && npx tsc -b && npm test

test-race:
	CGO_ENABLED=1 go test -race ./...

lint: ## vet and lint the Go code
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
