# starkite Makefile

BINARY_NAME=kite
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags "-X github.com/vladimirvivien/starkite/core/version.Version=$(VERSION) -X github.com/vladimirvivien/starkite/core/version.BuildTime=$(BUILD_TIME)"
CLOUD_LDFLAGS=-ldflags "-X github.com/vladimirvivien/starkite/core/version.Version=$(VERSION) -X github.com/vladimirvivien/starkite/core/version.BuildTime=$(BUILD_TIME) -X github.com/vladimirvivien/starkite/core/version.Edition=cloud"
AI_LDFLAGS=-ldflags "-X github.com/vladimirvivien/starkite/core/version.Version=$(VERSION) -X github.com/vladimirvivien/starkite/core/version.BuildTime=$(BUILD_TIME) -X github.com/vladimirvivien/starkite/core/version.Edition=ai"

.PHONY: all build build-core build-cloud build-ai clean test test-starbase test-core test-cloud test-ai install deps lint fmt help

all: deps build ## Build after fetching dependencies

build: build-core build-cloud build-ai ## Build all editions

build-core: ## Build the base edition binary
	cd core && go build $(LDFLAGS) -o ../$(BINARY_NAME) .

build-cloud: ## Build the cloud edition binary
	cd cloud && go build $(CLOUD_LDFLAGS) -o ../$(BINARY_NAME)-cloud .

build-ai: ## Build the ai edition binary
	cd ai && go build $(AI_LDFLAGS) -o ../$(BINARY_NAME)-ai .

clean: ## Remove build artifacts
	rm -f $(BINARY_NAME) $(BINARY_NAME)-cloud $(BINARY_NAME)-ai
	rm -rf dist/

test: test-starbase test-core test-cloud test-ai ## Run all tests

test-starbase: ## Run starbase tests
	cd starbase && go test ./...

test-core: ## Run core tests
	cd core && go test ./...

test-cloud: ## Run cloud tests
	cd cloud && go test ./...

test-ai: ## Run ai tests
	cd ai && go test ./...

install: build-core ## Install base edition to GOPATH/bin
	cd core && go install $(LDFLAGS) .

deps: ## Download dependencies
	cd starbase && go mod tidy
	cd core && go mod tidy
	cd cloud && go mod tidy
	cd ai && go mod tidy

lint: ## Run linter
	cd starbase && golangci-lint run ./...
	cd core && golangci-lint run ./...
	cd cloud && golangci-lint run ./...
	cd ai && golangci-lint run ./...

fmt: ## Format code
	cd starbase && go fmt ./...
	cd core && go fmt ./...
	cd cloud && go fmt ./...
	cd ai && go fmt ./...

run-example: build-core ## Run hello example
	./$(BINARY_NAME) run examples/core/hello.star

repl: build-core ## Start interactive REPL
	./$(BINARY_NAME) repl

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
