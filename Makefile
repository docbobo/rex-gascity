BIN_DIR ?= bin
BIN := $(BIN_DIR)/gc-runtime-rex
CITY_DIR ?= testdata/city
GC_REX_CITY ?= rex-verify

# The version the pack declares, stamped into the binary so the rex-gascity-runtime:rex-binary
# doctor check can compare the installed provider against pack.toml. A build
# without this stamp reports its module pseudo-version, which will not match.
PACK_VERSION ?= $(shell awk -F'"' '/^\[pack\]/{p=1;next} /^\[/{p=0} p && /^version[[:space:]]*=/{print $$2; exit}' pack/pack.toml)
LDFLAGS ?= -X main.buildVersion=$(PACK_VERSION)

.PHONY: help build install hooks adr vet test bench test-e2e verify check conformance timings clean

help: ## List available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-14s %s\n", $$1, $$2}' Makefile

build: ## Build bin/gc-runtime-rex
	go build -ldflags "$(LDFLAGS)" -o "$(BIN)" ./cmd/gc-runtime-rex

install: ## Install gc-runtime-rex with go install
	go install -ldflags "$(LDFLAGS)" ./cmd/gc-runtime-rex

hooks: ## Enable the versioned git hooks in .githooks
	git config core.hooksPath .githooks

adr: ## Write the next docs/adr/NNNN-<slug>.md from the template (TITLE="...")
	@sh scripts/new-adr.sh "$(TITLE)"

vet: ## Run go vet across all packages
	go vet ./...

test: ## Run unit tests across all packages
	go test ./...

bench: ## Run Go micro benchmarks (no tests); compare runs with benchstat
	go test -run '^$$' -bench . -benchmem ./internal/...

test-e2e: ## Run the fake-backed e2e suites (no rex on PATH needed)
	REX_E2E=fake go test ./internal/provider ./cmd/gc-runtime-rex -run TestE2E -v
	sh pack/tests/check-import.sh

verify: ## Run the pack check scripts (pack/tests/verify.sh)
	bash pack/tests/verify.sh

check: build ## Build and run gc runtime check against testdata/city
	PATH="$(abspath $(BIN_DIR)):$$PATH" GC_REX_CITY="$(GC_REX_CITY)" gc runtime check rex --city "$(CITY_DIR)"

conformance: build ## Build and run gc runtime conformance in a scratch city
	./pack/tests/conformance.sh --binary "$(abspath $(BIN))"

timings: ## Report p50 and p90 CI job wall time for the last 30 days
	sh scripts/ci-timings.sh

clean: ## Remove build artifacts
	rm -rf "$(BIN_DIR)"
