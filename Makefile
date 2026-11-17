BIN_DIR ?= bin
BIN := $(BIN_DIR)/gc-runtime-rex
CITY_DIR ?= testdata/city
GC_REX_CITY ?= rex-verify

.PHONY: help build install hooks vet test check conformance clean

help: ## List available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-14s %s\n", $$1, $$2}' Makefile

build: ## Build bin/gc-runtime-rex
	go build -o "$(BIN)" ./cmd/gc-runtime-rex

install: ## Install gc-runtime-rex with go install
	go install ./cmd/gc-runtime-rex

hooks: ## Enable the versioned git hooks in .githooks
	git config core.hooksPath .githooks

vet: ## Run go vet across all packages
	go vet ./...

test: ## Run unit tests across all packages
	go test ./...

check: build ## Build and run gc runtime check against testdata/city
	PATH="$(abspath $(BIN_DIR)):$$PATH" GC_REX_CITY="$(GC_REX_CITY)" gc runtime check rex --city "$(CITY_DIR)"

conformance: build ## Build and run gc runtime conformance against testdata/city
	PATH="$(abspath $(BIN_DIR)):$$PATH" GC_REX_CITY="$(GC_REX_CITY)" gc runtime conformance rex --city "$(CITY_DIR)"

clean: ## Remove build artifacts
	rm -rf "$(BIN_DIR)"
