BINARY  := revision
PKG     := ./cmd/revision
DIST    := dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# Every tape in docs/tapes except the settings-and-setup fragment they all source.
TAPES   := $(filter-out docs/tapes/common.tape,$(wildcard docs/tapes/*.tape))

.PHONY: all build run run-gallery site-data demos test cover vet fmt lint tidy cross build-darwin build-linux clean

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

## lint: run golangci-lint (must be installed)
lint:
	golangci-lint run

## tidy: tidy go.mod/go.sum
tidy:
	go mod tidy

## build-darwin: static macOS arm64 binary into ./dist
build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-darwin-arm64 $(PKG)

## build-linux: static Linux amd64 binary into ./dist
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY)-linux-amd64 $(PKG)

## cross: build both release binaries
cross: build-darwin build-linux

## clean: remove build artifacts
clean:
	rm -rf bin $(DIST) coverage.out coverage.html
