# starkite Makefile

BIN_DIR=bin
BINARY_NAME=kite
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
BASE_LDFLAGS=-ldflags "-X github.com/project-starkite/starkite/base/version.Version=$(VERSION) -X github.com/project-starkite/starkite/base/version.BuildTime=$(BUILD_TIME)"
CLOUD_LDFLAGS=-ldflags "-X github.com/project-starkite/starkite/base/version.Version=$(VERSION) -X github.com/project-starkite/starkite/base/version.BuildTime=$(BUILD_TIME) -X github.com/project-starkite/starkite/base/version.Edition=cloud"
AI_LDFLAGS=-ldflags "-X github.com/project-starkite/starkite/base/version.Version=$(VERSION) -X github.com/project-starkite/starkite/base/version.BuildTime=$(BUILD_TIME) -X github.com/project-starkite/starkite/base/version.Edition=ai"
ALL_LDFLAGS=-ldflags "-X github.com/project-starkite/starkite/base/version.Version=$(VERSION) -X github.com/project-starkite/starkite/base/version.BuildTime=$(BUILD_TIME) -X github.com/project-starkite/starkite/base/version.Edition=all"

.PHONY: all build build-base build-cloud build-ai build-all clean test test-libkite test-base test-cloud test-ai test-all install deps lint fmt help

all: deps build ## Build after fetching dependencies

build: build-base build-cloud build-ai build-all ## Build all editions into ./bin/

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build-base: $(BIN_DIR) ## Build the base edition binary (kitecmd)
	cd base && go build $(BASE_LDFLAGS) -o ../$(BIN_DIR)/kitecmd .

build-cloud: $(BIN_DIR) ## Build the cloud edition binary (kitecloud)
	cd cloud && go build $(CLOUD_LDFLAGS) -o ../$(BIN_DIR)/kitecloud .

build-ai: $(BIN_DIR) ## Build the ai edition binary (kiteai)
	cd ai && go build $(AI_LDFLAGS) -o ../$(BIN_DIR)/kiteai .

build-all: $(BIN_DIR) ## Build the all-in-one edition binary (kite)
	cd allkite && go build $(ALL_LDFLAGS) -o ../$(BIN_DIR)/$(BINARY_NAME) .

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)/ dist/

test: test-libkite test-base test-cloud test-ai test-all ## Run all tests

test-libkite: ## Run libkite (runtime) tests
	cd libkite && go test ./...

test-base: ## Run base tests
	cd base && go test ./...

test-cloud: ## Run cloud tests
	cd cloud && go test ./...

test-ai: ## Run ai tests
	cd ai && go test ./...

test-all: ## Run all-edition tests (registry composition guard)
	cd allkite && go test ./...

install: build-base ## Install base edition (kitecmd) to GOPATH/bin
	cd base && go install $(BASE_LDFLAGS) .

deps: ## Download dependencies
	cd libkite && go mod tidy
	cd base && go mod tidy
	cd cloud && go mod tidy
	cd ai && go mod tidy
	cd allkite && go mod tidy

lint: ## Run linter
	cd libkite && golangci-lint run ./...
	cd base && golangci-lint run ./...
	cd cloud && golangci-lint run ./...
	cd ai && golangci-lint run ./...
	cd allkite && golangci-lint run ./...

fmt: ## Format code
	cd libkite && go fmt ./...
	cd base && go fmt ./...
	cd cloud && go fmt ./...
	cd ai && go fmt ./...
	cd allkite && go fmt ./...

run-example: build-base ## Run hello example
	./$(BIN_DIR)/kitecmd run examples/core/hello.star

repl: build-base ## Start interactive REPL
	./$(BIN_DIR)/kitecmd repl

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
