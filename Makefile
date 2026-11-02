# starkite Makefile

BIN_DIR=bin
BINARY_NAME=kite
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
COMMON_LDFLAGS=-s -w -X github.com/project-starkite/starkite/basekite/version.Version=$(VERSION) -X github.com/project-starkite/starkite/basekite/version.BuildTime=$(BUILD_TIME) -X github.com/project-starkite/starkite/basekite/version.GitCommit=$(GIT_COMMIT) -X github.com/project-starkite/starkite/libkite/modules/runtime.Version=$(VERSION)
BASE_LDFLAGS=-ldflags "$(COMMON_LDFLAGS)"
CLOUD_LDFLAGS=-ldflags "$(COMMON_LDFLAGS) -X github.com/project-starkite/starkite/basekite/version.Edition=cloud"
AI_LDFLAGS=-ldflags "$(COMMON_LDFLAGS) -X github.com/project-starkite/starkite/basekite/version.Edition=ai"
ALL_LDFLAGS=-ldflags "$(COMMON_LDFLAGS) -X github.com/project-starkite/starkite/basekite/version.Edition=all"

KITE_TAGS=grammar_subset,grammar_subset_starlark

MODULES=libkite basekite cloudkite aikite kite tests/sandbox tests/dogfood

.PHONY: kite all build-base build-cloud build-ai clean test test-libkite test-base test-cloud test-ai test-all test-lsp test-lint test-dogfood lint-scripts package-vscode test-install-script install deps lint fmt fix vet test-race preflight run-example repl help


kite: $(BIN_DIR) ## Build the default all-in-one binary with embedded language server (kite)
	cd kite && go build -tags '$(KITE_TAGS)' $(ALL_LDFLAGS) -o ../$(BIN_DIR)/$(BINARY_NAME) .

all: kite build-base build-cloud build-ai ## Build all editions into ./bin/

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

fix: ## Run go fix AST modernization across all workspace modules (linux, darwin, windows)
	@for dir in $(MODULES); do \
		echo "=== Fixing $$dir (linux/darwin/windows) ==="; \
		for os in linux darwin windows; do \
			(cd $$dir && CGO_ENABLED=0 GOOS=$$os GOARCH=amd64 go fix ./...) || exit 1; \
		done; \
	done

fmt: fix ## Run go fix, go fmt, and verify formatting across all files
	@for dir in $(MODULES); do \
		(cd $$dir && go fmt ./...); \
	done
	@(cd libkite/lsp && go fmt ./...)
	@DIFFS=$$(gofmt -l .); \
	if [ -n "$$DIFFS" ]; then \
		echo "Formatting differences found in the following files:"; \
		echo "$$DIFFS"; \
		exit 1; \
	fi

vet: ## Run go vet across all workspace modules (linux, darwin, windows)
	@for dir in $(MODULES); do \
		echo "=== Vetting $$dir (linux/darwin/windows) ==="; \
		for os in linux darwin windows; do \
			(cd $$dir && CGO_ENABLED=0 GOOS=$$os GOARCH=amd64 go vet ./...) || exit 1; \
		done; \
	done
	@(cd libkite/lsp && go vet ./...)

test-race: ## Run all Go unit tests with race detection
	@for dir in $(MODULES); do \
		echo "=== Testing $$dir (race) ==="; \
		(cd $$dir && go test -race ./...); \
	done

lint: ## Run staticcheck across all workspace packages
	@PATH="$$(go env GOPATH)/bin:$$PATH" staticcheck ./libkite/... ./basekite/... ./cloudkite/... ./aikite/... ./kite/... ./tests/sandbox/...

generate-api-docs: ## Regenerate libkite/catalog/apidocs/table_gen.go from docs/references/api/*.md
	go run ./libkite/catalog/internal/gendocs

check-api-docs: ## Verify table_gen.go is in sync with docs/references/api/*.md
	go run ./libkite/catalog/internal/gendocs -check

generate-agent-skills: kite ## Generate in-tree .agent/skills/ definitions using kite doc --skills
	@mkdir -p .agent/skills/starkite .agent/skills/starkite-k8s .agent/skills/starkite-cloud
	@echo "Generating .agent/skills/starkite/SKILL.md..."
	@./$(BIN_DIR)/$(BINARY_NAME) doc --skills > .agent/skills/starkite/SKILL.md
	@echo "Generating .agent/skills/starkite-k8s/SKILL.md..."
	@./$(BIN_DIR)/$(BINARY_NAME) doc k8s --skills > .agent/skills/starkite-k8s/SKILL.md
	@echo "Generating .agent/skills/starkite-cloud/SKILL.md..."
	@./$(BIN_DIR)/$(BINARY_NAME) doc --skills --category=cloud > .agent/skills/starkite-cloud/SKILL.md
	@echo "=== Agent skills generated successfully ==="

check-agent-skills: kite ## Verify in-tree agent skills are synchronized with runtime catalog
	@CHECK_DIR=$$(mktemp -d); \
	mkdir -p "$$CHECK_DIR/starkite" "$$CHECK_DIR/starkite-k8s" "$$CHECK_DIR/starkite-cloud"; \
	./$(BIN_DIR)/$(BINARY_NAME) doc --skills > "$$CHECK_DIR/starkite/SKILL.md"; \
	./$(BIN_DIR)/$(BINARY_NAME) doc k8s --skills > "$$CHECK_DIR/starkite-k8s/SKILL.md"; \
	./$(BIN_DIR)/$(BINARY_NAME) doc --skills --category=cloud > "$$CHECK_DIR/starkite-cloud/SKILL.md"; \
	diff -u .agent/skills/starkite/SKILL.md "$$CHECK_DIR/starkite/SKILL.md" || (echo "ERROR: .agent/skills/starkite/SKILL.md out of sync. Run 'make generate-agent-skills'"; rm -rf "$$CHECK_DIR"; exit 1); \
	diff -u .agent/skills/starkite-k8s/SKILL.md "$$CHECK_DIR/starkite-k8s/SKILL.md" || (echo "ERROR: .agent/skills/starkite-k8s/SKILL.md out of sync. Run 'make generate-agent-skills'"; rm -rf "$$CHECK_DIR"; exit 1); \
	diff -u .agent/skills/starkite-cloud/SKILL.md "$$CHECK_DIR/starkite-cloud/SKILL.md" || (echo "ERROR: .agent/skills/starkite-cloud/SKILL.md out of sync. Run 'make generate-agent-skills'"; rm -rf "$$CHECK_DIR"; exit 1); \
	rm -rf "$$CHECK_DIR"; \
	echo "=== In-tree agent skills verified in sync ==="

preflight: check-api-docs check-agent-skills fix fmt vet test-race lint test-lint lint-scripts ## Run complete Go code hygiene and pre-flight verification suite
	@echo "=== Pre-flight verification passed cleanly ==="

test-lint: ## Run unit tests for static analysis and dialect linter
	cd libkite/lint && go test ./...
	cd libkite/lsp && go test -run TestLinter ./...
	cd basekite && go test -run TestValidate ./cmd

test-dogfood: kite ## Run autonomous agent closed-loop dogfooding test harness
	go test -v ./tests/dogfood/...

lint-scripts: kite ## Validate all in-tree Starlark test and example scripts
	@echo "Validating all examples/*.star..."
	@find examples -name "*.star" -exec ./$(BIN_DIR)/$(BINARY_NAME) validate --lint {} +
	@echo "All example scripts validated cleanly."

package-vscode: ## Verify packaging of VS Code extension into .vsix
	@if command -v npx >/dev/null 2>&1; then \
		echo "Packaging VS Code extension..."; \
		(cd editors/vscode && npm install --silent && npx @vscode/vsce package --no-dependencies); \
	else \
		echo "npx not found, skipping vsce package"; \
	fi


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

test-lsp: ## Run all tests for the embedded language server
	cd libkite/lsp && go test ./...
	cd basekite && go test -run TestLSP ./cmd

test-install-script: ## Dry-run test the installer script
	@echo "Testing install.sh script in dry-run mode..."
	@INSTALL_DRY_RUN=1 ./scripts/install.sh

install: kite ## Install the default kite binary to GOPATH/bin
	cd kite && go install -tags '$(KITE_TAGS)' $(ALL_LDFLAGS) .

deps: ## Download dependencies
	cd libkite && go mod tidy
	cd basekite && go mod tidy
	cd cloudkite && go mod tidy
	cd aikite && go mod tidy
	cd kite && go mod tidy

IMAGE_NAME?=ghcr.io/project-starkite/starkite:latest

kite-linux: ## Build static Linux kite binary (for container image packaging)
	mkdir -p $(BIN_DIR)
	cd kite && CGO_ENABLED=0 GOOS=linux go build -tags '$(KITE_TAGS)' $(ALL_LDFLAGS) -o ../$(BIN_DIR)/kite-linux .

docker-build: kite-linux ## Build the starkite container image using Docker
	docker build -t $(IMAGE_NAME) -f Dockerfile .

podman-build: kite-linux ## Build the starkite container image using Podman
	podman build -t $(IMAGE_NAME) -f Dockerfile .

run-example: kite ## Run hello example
	./$(BIN_DIR)/kite run examples/core/hello.star

repl: kite ## Start interactive REPL
	./$(BIN_DIR)/kite repl

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := kite

