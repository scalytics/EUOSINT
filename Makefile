SHELL := /bin/bash

NODE_VERSION := 25.8.1
NPM_VERSION := 11.11.0
GO_VERSION := 1.25.2
GO_COVERAGE_THRESHOLD ?= 45
GO_COVER_PROFILE ?= .tmp/go-coverage.out
GO_COVER_REPORT ?= .tmp/go-coverage.txt
GOCACHE_DIR ?= $(CURDIR)/.tmp/go-build-cache
GOMODCACHE_DIR ?= $(CURDIR)/.tmp/go-mod-cache
CODEQL_RAM_MB ?= 4096
DOCKER_IMAGE ?= euosint
IMAGE_TAG ?= local
BUILDER ?= colima
DOCKER_COMPOSE ?= $(shell if command -v docker-compose >/dev/null 2>&1; then echo docker-compose; else echo "docker compose"; fi)
CODEQL_DIR ?= .tmp/codeql
CODEQL_JS_DB ?= $(CODEQL_DIR)/js-db
CODEQL_GO_DB ?= $(CODEQL_DIR)/go-db
CODEQL_JS_OUT ?= $(CODEQL_DIR)/javascript.sarif
CODEQL_GO_OUT ?= $(CODEQL_DIR)/go.sarif
BRANCH ?= main
RELEASE_LEVEL ?= patch

.PHONY: help check check-commit install clean lint typecheck test build ci \
	go-fmt go-fmt-check go-test go-race go-cover go-vet go-codeql commit-check \
	docker-build docker-up docker-down docker-logs docker-shell \
	dev-start dev-stop dev-restart dev-logs \
	code-ql code-ql-summary \
	release-patch release-minor release-major \
	branch-protection

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2} END {printf "\n"}' $(MAKEFILE_LIST)

check: ## Validate local toolchain
	@echo "Checking prerequisites..."
	@command -v node >/dev/null 2>&1 || { echo "Node.js is required"; exit 1; }
	@command -v npm >/dev/null 2>&1 || { echo "npm is required"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "docker is required"; exit 1; }
	@command -v gh >/dev/null 2>&1 || { echo "gh is required for release/branch targets"; exit 1; }
	@docker compose version >/dev/null 2>&1 || command -v docker-compose >/dev/null 2>&1 || { echo "docker compose or docker-compose is required"; exit 1; }
	@echo "  Node $$(node -v) — expected $(NODE_VERSION)"
	@echo "  npm $$(npm -v) — expected $(NPM_VERSION)"
	@echo "  docker $$(docker --version | sed 's/Docker version //; s/,.*//')"
	@echo "  compose $$($(DOCKER_COMPOSE) version 2>/dev/null | head -n 1)"
	@echo "  gh $$(gh --version | head -n 1 | sed 's/gh version //')"

check-commit: ## Validate local toolchain for commit checks
	@echo "Checking commit prerequisites..."
	@command -v node >/dev/null 2>&1 || { echo "Node.js is required"; exit 1; }
	@command -v npm >/dev/null 2>&1 || { echo "npm is required"; exit 1; }
	@command -v go >/dev/null 2>&1 || { echo "Go is required"; exit 1; }
	@command -v codeql >/dev/null 2>&1 || { echo "codeql CLI is required"; exit 1; }
	@echo "  Node $$(node -v) — expected $(NODE_VERSION)"
	@echo "  npm $$(npm -v) — expected $(NPM_VERSION)"
	@echo "  go $$(go version | awk '{print $$3}') — expected go$(GO_VERSION)"
	@echo "  codeql $$(codeql version | head -n 1 | awk '{print $$5}')"

install: ## Install project dependencies
	npm install

clean: ## Remove build and temporary artifacts
	rm -rf dist coverage .tmp

lint: ## Run ESLint
	npm run lint

typecheck: ## Run TypeScript type checks
	npm run typecheck

test: ## Run repository test suite
	npm test

build: ## Build the production bundle
	npm run build

ci: check lint test build ## Run the full local CI suite

go-fmt: ## Auto-format Go code
	@mkdir -p $(GOCACHE_DIR) $(GOMODCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) GOMODCACHE=$(GOMODCACHE_DIR) gofmt -w $$(find cmd internal -name '*.go' -type f | sort)

go-fmt-check: ## Fail if Go files are not formatted
	@unformatted=$$(gofmt -l $$(find cmd internal -name '*.go' -type f | sort)); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needs to be run for:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

go-test: ## Run Go tests
	@mkdir -p $(GOCACHE_DIR) $(GOMODCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) GOMODCACHE=$(GOMODCACHE_DIR) go test ./...

go-race: ## Run Go race detector
	@mkdir -p $(GOCACHE_DIR) $(GOMODCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) GOMODCACHE=$(GOMODCACHE_DIR) go test -race ./...

go-cover: ## Enforce Go coverage threshold
	@mkdir -p .tmp $(GOCACHE_DIR) $(GOMODCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) GOMODCACHE=$(GOMODCACHE_DIR) go test -covermode=atomic -coverprofile=$(GO_COVER_PROFILE) ./...
	GOCACHE=$(GOCACHE_DIR) go tool cover -func=$(GO_COVER_PROFILE) | tee $(GO_COVER_REPORT)
	@coverage=$$(GOCACHE=$(GOCACHE_DIR) go tool cover -func=$(GO_COVER_PROFILE) | awk '/^total:/ {gsub("%","",$$3); print $$3}'); \
	awk -v coverage="$$coverage" -v threshold="$(GO_COVERAGE_THRESHOLD)" 'BEGIN { if (coverage + 0 < threshold + 0) { printf("coverage %.1f%% is below threshold %.1f%%\n", coverage, threshold); exit 1 } }'

go-vet: ## Run go vet
	@mkdir -p $(GOCACHE_DIR) $(GOMODCACHE_DIR)
	GOCACHE=$(GOCACHE_DIR) GOMODCACHE=$(GOMODCACHE_DIR) go vet ./...

docker-build: ## Build the Docker image with buildx
	docker buildx build --builder $(BUILDER) --load -t $(DOCKER_IMAGE):$(IMAGE_TAG) .

docker-up: ## Start the Docker stack
	$(DOCKER_COMPOSE) up --build -d

docker-down: ## Stop the Docker stack
	$(DOCKER_COMPOSE) down --remove-orphans

docker-logs: ## Tail Docker logs
	$(DOCKER_COMPOSE) logs -f --tail=200

docker-shell: ## Open a shell in the running container
	$(DOCKER_COMPOSE) exec euosint sh

dev-start: ## Start the local HTTP dev stack on localhost
	$(DOCKER_COMPOSE) up --build -d
	@echo "EUOSINT available at http://localhost:$${EUOSINT_HTTP_PORT:-8080}"
	@open "http://localhost:$${EUOSINT_HTTP_PORT:-8080}"

dev-stop: ## Stop the local dev stack and prune dangling images
	$(DOCKER_COMPOSE) down --remove-orphans
	@docker image prune -f --filter "label=com.docker.compose.project" >/dev/null 2>&1 || true
	@docker builder prune -f >/dev/null 2>&1 || true

dev-restart: ## Restart the local dev stack (prunes old images first)
	$(DOCKER_COMPOSE) down --remove-orphans
	@docker image prune -f --filter "label=com.docker.compose.project" >/dev/null 2>&1 || true
	@docker builder prune -f >/dev/null 2>&1 || true
	$(DOCKER_COMPOSE) up --build -d
	@echo "EUOSINT available at http://localhost:$${EUOSINT_HTTP_PORT:-8080}"
	@open "http://localhost:$${EUOSINT_HTTP_PORT:-8080}"

dev-sync-registry: ## Merge source_registry.json into the running DB (adds new feeds)
	$(DOCKER_COMPOSE) exec collector euosint-collector --source-db /data/sources.db --curated-seed /app/registry/source_registry.json --source-db-merge-registry

dev-logs: ## Tail local dev stack logs
	$(DOCKER_COMPOSE) logs -f --tail=200

code-ql: ## Run CodeQL CLI locally for JavaScript/TypeScript
	@command -v codeql >/dev/null 2>&1 || { echo "codeql CLI is required"; exit 1; }
	rm -rf $(CODEQL_JS_DB)
	mkdir -p $(CODEQL_DIR)
	codeql database create $(CODEQL_JS_DB) \
		--language=javascript-typescript \
		--source-root=. \
		--command="npm ci && npm run build"
	codeql database analyze $(CODEQL_JS_DB) \
		codeql/javascript-queries:codeql-suites/javascript-security-and-quality.qls \
		--ram=$(CODEQL_RAM_MB) \
		--format=sarif-latest \
		--output=$(CODEQL_JS_OUT)
	@echo "Wrote $(CODEQL_JS_OUT)"

go-codeql: ## Run CodeQL CLI locally for Go
	@command -v codeql >/dev/null 2>&1 || { echo "codeql CLI is required"; exit 1; }
	rm -rf $(CODEQL_GO_DB)
	mkdir -p $(CODEQL_DIR)
	codeql database create $(CODEQL_GO_DB) \
		--language=go \
		--source-root=. \
		--command="env GOCACHE=$(GOCACHE_DIR) GOMODCACHE=$(GOMODCACHE_DIR) go build ./cmd/euosint-collector"
	codeql database analyze $(CODEQL_GO_DB) \
		codeql/go-queries:codeql-suites/go-security-and-quality.qls \
		--ram=$(CODEQL_RAM_MB) \
		--format=sarif-latest \
		--output=$(CODEQL_GO_OUT)
	@echo "Wrote $(CODEQL_GO_OUT)"

code-ql-summary: ## Summarize the local CodeQL SARIF output
	python3 scripts/codeql_summary.py $(CODEQL_JS_OUT)

commit-check: ## Run the full local quality gate with auto-formatting
	@set -euo pipefail; \
	steps=( \
		"check-commit:toolchain" \
		"go-fmt:go format" \
		"lint:ui lint" \
		"typecheck:ui typecheck" \
		"build:ui build" \
		"go-test:go test" \
		"go-race:go race" \
		"go-cover:go coverage" \
		"go-vet:go vet" \
		"code-ql:js codeql" \
		"go-codeql:go codeql" \
	); \
		total=$${#steps[@]}; \
		index=0; \
		for entry in "$${steps[@]}"; do \
			index=$$((index + 1)); \
			target=$${entry%%:*}; \
			label=$${entry#*:}; \
			printf '\n[%d/%d] %s\n' "$$index" "$$total" "$$label"; \
			$(MAKE) --no-print-directory "$$target"; \
			printf '[ok] %s\n' "$$label"; \
		done; \
		if ! git diff --quiet -- cmd internal; then \
			printf '\n[info] gofmt rewrote Go files under cmd/ or internal/\n'; \
		fi; \
		printf '\n[done] commit-check passed\n'

release-patch: ## Create and push the next patch release tag
	bash scripts/release-tag.sh patch

release-minor: ## Create and push the next minor release tag
	bash scripts/release-tag.sh minor

release-major: ## Create and push the next major release tag
	bash scripts/release-tag.sh major

branch-protection: ## Apply branch protection to the configured branch
	bash scripts/apply-branch-protection.sh $(BRANCH)
