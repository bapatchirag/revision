BINARY  := revision
PKG     := ./cmd/revision
DIST    := dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# Every tape in docs/tapes except the settings-and-setup fragment they all source.
TAPES   := $(filter-out docs/tapes/common.tape,$(wildcard docs/tapes/*.tape))

.PHONY: all build run run-gallery site-data site-changelog site-build site-dev demos test cover vet fmt lint tidy cross build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64 clean

all: build

## build: compile the binary for the host platform into ./bin
build:
	go build -ldflags '$(LDFLAGS)' -o bin/$(BINARY) $(PKG)

## run: run the TUI from source
run:
	go run $(PKG)

## run-gallery: run the reusable-component gallery from source
run-gallery:
	go run ./cmd/gallery

## site-data: regenerate the website's keybindings table from the Go source
site-data:
	go run ./cmd/keymapdump > site/src/data/keybindings.json

## site-changelog: regenerate the website's release history from the GitHub releases
site-changelog:
	npm --prefix site run changelog

## site-build: regenerate the keybindings table, then build the website into site/dist
site-build: site-data
	npm --prefix site ci && npm --prefix site run build

## site-dev: serve the website locally with hot reload (Pagefind search needs site-build)
site-dev:
	npm --prefix site install && npm --prefix site run dev

## demos: re-record every per-feature demo and derive the poster frames
demos:
	@for tape in $(TAPES); do echo "vhs $$tape"; vhs $$tape; done
	npm --prefix site run demo

## test: run all unit tests
test:
	go test ./...

## cover: run tests with a coverage summary
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

## vet: run go vet
vet:
	go vet ./...

## fmt: format all Go sources
fmt:
	gofmt -l -w .

## lint: run golangci-lint (must be installed) and shellcheck (if installed)
lint:
	golangci-lint run
	@if command -v shellcheck >/dev/null 2>&1; then shellcheck install.sh; else echo "shellcheck not installed; skipping install.sh (CI runs it)"; fi

## tidy: tidy go.mod/go.sum
tidy:
	go mod tidy

## build-darwin-arm64: static macOS arm64 binary into ./dist
build-darwin-arm64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-darwin-arm64 $(PKG)

## build-darwin-amd64: static macOS amd64 binary into ./dist
build-darwin-amd64:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-darwin-amd64 $(PKG)

## build-linux-amd64: static Linux amd64 binary into ./dist
build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-linux-amd64 $(PKG)

## build-linux-arm64: static Linux arm64 binary into ./dist
build-linux-arm64:
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-linux-arm64 $(PKG)

## cross: build every release binary
cross: build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64

## clean: remove build artifacts
clean:
	rm -rf bin $(DIST) coverage.out coverage.html
