BINARY      := cronus
PKG         := github.com/t0mer/cronus
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X $(PKG)/internal/version.Version=$(VERSION)
GO          ?= go

.PHONY: build run test lint web docker release-dry tidy clean

## build: compile the static binary into dist/
build: web
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY) ./cmd/$(BINARY)

## run: run the server locally
run:
	$(GO) run ./cmd/$(BINARY) serve

## test: run the Go test suite with the race detector
test:
	$(GO) test ./... -race

## lint: run go vet and golangci-lint if present
lint:
	$(GO) vet ./...
	@which golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed; ran go vet only"

## web: build the embedded frontend (no-op until web/ exists with a build)
web:
	@if [ -f web/package.json ]; then \
		cd web && npm ci && npm run build; \
	else \
		echo "no web/package.json yet; skipping frontend build"; \
	fi

## docker: build the multi-arch-capable image locally (single arch)
docker:
	docker build --build-arg VERSION=$(VERSION) -t techblog/$(BINARY):$(VERSION) .

## release-dry: run goreleaser in snapshot mode
release-dry:
	goreleaser release --snapshot --clean --skip=publish

## tidy: tidy go modules
tidy:
	$(GO) mod tidy

## clean: remove build artifacts
clean:
	rm -rf dist web/dist
