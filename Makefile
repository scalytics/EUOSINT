SHELL := /bin/bash

NODE_VERSION := 25.8.1
NPM_VERSION := 11.11.0
DOCKER_IMAGE ?= euosint
IMAGE_TAG ?= local
BUILDER ?= colima
DOCKER_COMPOSE ?= $(shell if command -v docker-compose >/dev/null 2>&1; then echo docker-compose; else echo "docker compose"; fi)
CODEQL_DIR ?= .tmp/codeql
CODEQL_DB ?= $(CODEQL_DIR)/db
CODEQL_OUT ?= $(CODEQL_DIR)/javascript.sarif
BRANCH ?= main
RELEASE_LEVEL ?= patch

.PHONY: help check install clean lint typecheck test build ci \
	docker-build docker-up docker-down docker-logs docker-shell \
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

code-ql: ## Run CodeQL CLI locally for JavaScript/TypeScript
	@command -v codeql >/dev/null 2>&1 || { echo "codeql CLI is required"; exit 1; }
	rm -rf $(CODEQL_DIR)
	mkdir -p $(CODEQL_DIR)
	codeql database create $(CODEQL_DB) \
		--language=javascript-typescript \
		--source-root=. \
		--command="npm ci && npm run build"
	codeql database analyze $(CODEQL_DB) \
		javascript-security-and-quality.qls \
		--format=sarif-latest \
		--output=$(CODEQL_OUT)
	@echo "Wrote $(CODEQL_OUT)"

code-ql-summary: ## Summarize the local CodeQL SARIF output
	python3 scripts/codeql_summary.py $(CODEQL_OUT)

release-patch: ## Create and push the next patch release tag
	bash scripts/release-tag.sh patch

release-minor: ## Create and push the next minor release tag
	bash scripts/release-tag.sh minor

release-major: ## Create and push the next major release tag
	bash scripts/release-tag.sh major

branch-protection: ## Apply branch protection to the configured branch
	bash scripts/apply-branch-protection.sh $(BRANCH)
