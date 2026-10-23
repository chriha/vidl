BINARY  := vidl
PKG     := ./cmd/vidl
BIN_DIR := bin
DIST    := dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
PLATFORMS := darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/amd64 windows/arm64

.DEFAULT_GOAL := build
.PHONY: help build run install uninstall test race cover vet fmt fmt-check lint tidy check dist clean

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary into bin/
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(PKG)

run: build ## Build and run (pass URLs with ARGS="<url> ...")
	./$(BIN_DIR)/$(BINARY) $(ARGS)

install: ## Install into $(go env GOPATH)/bin
	go install -trimpath -ldflags "$(LDFLAGS)" $(PKG)

uninstall: ## Remove the installed binary
	rm -f "$$(go env GOPATH)/bin/$(BINARY)"

test: ## Run tests
	go test ./...

race: ## Run tests with the race detector
	go test -race ./...

cover: ## Run tests with a coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

vet: ## Run go vet
	go vet ./...

fmt: ## Format the code
	gofmt -w .

fmt-check: ## Fail if code is not formatted
	@test -z "$$(gofmt -l .)" || (gofmt -l .; echo "run 'make fmt'"; exit 1)

lint: ## Run golangci-lint if installed
	@command -v golangci-lint >/dev/null || { echo "golangci-lint not installed, skipping"; exit 0; }; golangci-lint run

tidy: ## Tidy go.mod and go.sum
	go mod tidy

check: fmt-check vet race ## Run all checks (formatting, vet, race tests)

dist: ## Cross-compile release binaries and checksums into dist/
	@rm -rf $(DIST) && mkdir -p $(DIST)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		out=$(DIST)/$(BINARY)-$(VERSION)-$$os-$$arch$$ext; \
		echo "building $$out"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $$out $(PKG) || exit 1; \
	done
	@cd $(DIST) && shasum -a 256 $(BINARY)-* > checksums.txt

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) $(DIST) coverage.out
