# starkite Makefile

BIN_DIR=bin
BINARY_NAME=kite
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
COMMON_LDFLAGS=-s -w -X github.com/project-starkite/starkite/basekite/version.Version=$(VERSION) -X github.com/project-starkite/starkite/basekite/version.BuildTime=$(BUILD_TIME) -X github.com/project-starkite/starkite/basekite/version.GitCommit=$(GIT_COMMIT)
BASE_LDFLAGS=-ldflags "$(COMMON_LDFLAGS)"
CLOUD_LDFLAGS=-ldflags "$(COMMON_LDFLAGS) -X github.com/project-starkite/starkite/basekite/version.Edition=cloud"
AI_LDFLAGS=-ldflags "$(COMMON_LDFLAGS) -X github.com/project-starkite/starkite/basekite/version.Edition=ai"
ALL_LDFLAGS=-ldflags "$(COMMON_LDFLAGS) -X github.com/project-starkite/starkite/basekite/version.Edition=all"

.PHONY: kite all build-base build-cloud build-ai clean test test-libkite test-base test-cloud test-ai test-all install deps lint fmt run-example repl help

kite: $(BIN_DIR) ## Build the default all-in-one binary (kite)
	cd kite && go build $(ALL_LDFLAGS) -o ../$(BIN_DIR)/$(BINARY_NAME) .

all: kite build-base build-cloud build-ai ## Build all four editions into ./bin/

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build-base: $(BIN_DIR) ## Build the lean base edition binary (kitecmd)
	cd basekite && go build $(BASE_LDFLAGS) -o ../$(BIN_DIR)/kitecmd .

build-cloud: $(BIN_DIR) ## Build the lean cloud edition binary (kitecloud)
	cd cloudkite && go build $(CLOUD_LDFLAGS) -o ../$(BIN_DIR)/kitecloud .

build-ai: $(BIN_DIR) ## Build the lean ai edition binary (kiteai)
	cd aikite && go build $(AI_LDFLAGS) -o ../$(BIN_DIR)/kiteai .

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)/ dist/

test: test-libkite test-base test-cloud test-ai test-all test-install-script ## Run all tests

test-libkite: ## Run libkite (runtime) tests
	cd libkite && go test ./...

test-base: ## Run base tests
	cd basekite && go test ./...

test-cloud: ## Run cloud tests
	cd cloudkite && go test ./...

test-ai: ## Run ai tests
	cd aikite && go test ./...

test-all: ## Run all-edition tests (registry composition guard)
	cd kite && go test ./...

test-install-script: ## Dry-run test the installer script
	@echo "Testing install.sh script in dry-run mode..."
	@INSTALL_DRY_RUN=1 ./scripts/install.sh

install: kite ## Install the default kite binary to GOPATH/bin
	cd kite && go install $(ALL_LDFLAGS) .

deps: ## Download dependencies
	cd libkite && go mod tidy
	cd basekite && go mod tidy
	cd cloudkite && go mod tidy
	cd aikite && go mod tidy
	cd kite && go mod tidy

run-example: kite ## Run hello example
	./$(BIN_DIR)/kite run examples/core/hello.star

repl: kite ## Start interactive REPL
	./$(BIN_DIR)/kite repl

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := kite
