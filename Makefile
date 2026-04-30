# starkite Makefile

BIN_DIR=bin
BINARY_NAME=kite
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
BASE_LDFLAGS=-ldflags "-X github.com/project-starkite/starkite/basekite/version.Version=$(VERSION) -X github.com/project-starkite/starkite/basekite/version.BuildTime=$(BUILD_TIME)"
CLOUD_LDFLAGS=-ldflags "-X github.com/project-starkite/starkite/basekite/version.Version=$(VERSION) -X github.com/project-starkite/starkite/basekite/version.BuildTime=$(BUILD_TIME) -X github.com/project-starkite/starkite/basekite/version.Edition=cloud"
AI_LDFLAGS=-ldflags "-X github.com/project-starkite/starkite/basekite/version.Version=$(VERSION) -X github.com/project-starkite/starkite/basekite/version.BuildTime=$(BUILD_TIME) -X github.com/project-starkite/starkite/basekite/version.Edition=ai"
ALL_LDFLAGS=-ldflags "-X github.com/project-starkite/starkite/basekite/version.Version=$(VERSION) -X github.com/project-starkite/starkite/basekite/version.BuildTime=$(BUILD_TIME) -X github.com/project-starkite/starkite/basekite/version.Edition=all"

.PHONY: all build build-base build-cloud build-ai build-all clean test test-starbase test-base test-cloud test-ai test-all install deps lint fmt help

all: deps build ## Build after fetching dependencies

build: build-base build-cloud build-ai build-all ## Build all editions into ./bin/

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build-base: $(BIN_DIR) ## Build the base edition binary
	cd basekite && go build $(BASE_LDFLAGS) -o ../$(BIN_DIR)/basekite .

build-cloud: $(BIN_DIR) ## Build the cloud edition binary
	cd cloudkite && go build $(CLOUD_LDFLAGS) -o ../$(BIN_DIR)/cloudkite .

build-ai: $(BIN_DIR) ## Build the ai edition binary
	cd aikite && go build $(AI_LDFLAGS) -o ../$(BIN_DIR)/aikite .

build-all: $(BIN_DIR) ## Build the all-in-one edition binary
	cd allkite && go build $(ALL_LDFLAGS) -o ../$(BIN_DIR)/$(BINARY_NAME) .

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)/ dist/

test: test-starbase test-base test-cloud test-ai test-all ## Run all tests

test-starbase: ## Run starbase tests
	cd starbase && go test ./...

test-base: ## Run base tests
	cd basekite && go test ./...

test-cloud: ## Run cloud tests
	cd cloudkite && go test ./...

test-ai: ## Run ai tests
	cd aikite && go test ./...

test-all: ## Run all-edition tests (registry composition guard)
	cd allkite && go test ./...

install: build-base ## Install base edition to GOPATH/bin
	cd basekite && go install $(BASE_LDFLAGS) .

deps: ## Download dependencies
	cd starbase && go mod tidy
	cd basekite && go mod tidy
	cd cloudkite && go mod tidy
	cd aikite && go mod tidy
	cd allkite && go mod tidy

lint: ## Run linter
	cd starbase && golangci-lint run ./...
	cd basekite && golangci-lint run ./...
	cd cloudkite && golangci-lint run ./...
	cd aikite && golangci-lint run ./...
	cd allkite && golangci-lint run ./...

fmt: ## Format code
	cd starbase && go fmt ./...
	cd basekite && go fmt ./...
	cd cloudkite && go fmt ./...
	cd aikite && go fmt ./...
	cd allkite && go fmt ./...

run-example: build-base ## Run hello example
	./$(BIN_DIR)/basekite run examples/core/hello.star

repl: build-base ## Start interactive REPL
	./$(BIN_DIR)/basekite repl

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
