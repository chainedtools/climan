# climan — Makefile
#
# Targets:
#   make build    compile ./bin/climan
#   make install  build and install to ~/.local/bin
#   make test     run unit tests
#   make lint     run golangci-lint (if installed)
#   make fmt      gofmt + goimports
#   make tidy     prune/go-get dependencies
#   make clean    remove build artifacts

BINARY  := climan
PKG     := go.solved.gg/climan
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(PKG)/internal/cli.Version=$(VERSION)

.PHONY: build install test lint fmt tidy clean run

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

install: build
	install -m 0755 bin/$(BINARY) $${HOME}/.local/bin/$(BINARY)
	@echo "installed $${HOME}/.local/bin/$(BINARY)"

run: build
	./bin/$(BINARY) $(ARGS)

test:
	go test ./...

lint:
	command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not found (nix shell provides it)"; exit 1; }
	golangci-lint run ./...

fmt:
	gofmt -w .
	command -v goimports >/dev/null 2>&1 && goimports -w . || true

tidy:
	go mod tidy

clean:
	rm -rf bin
