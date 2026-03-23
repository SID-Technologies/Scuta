##@ Core
.PHONY: help
help:  ## Display this help message.
	@echo "Usage:"
	@echo "  make [target]"
	@awk 'BEGIN {FS = ":.*?## "} \
		/^[a-zA-Z0-9_-]+:.*?## / { \
			printf "\033[36m  %-45s\033[0m %s\n", $$1, $$2 \
		} \
		/^##@/ { \
			printf "\n\033[1m%s\033[0m\n", substr($$0, 5) \
		}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the binary to dist/
	@mkdir -p dist
	@go build -ldflags="-X github.com/sid-technologies/scuta/cmd.version=$$(git describe --tags --always 2>/dev/null || echo dev)" -o dist/scuta .

.PHONY: clean
clean: ## Remove build artifacts
	@rm -rf dist/

.PHONY: install-tools
install-tools: ## Install pre-commit hooks and dependencies
	@which pre-commit > /dev/null || echo "pre-commit not installed, see https://pre-commit.com/#install"
	@pre-commit install --install-hooks

.PHONY: lint
lint: ## Run linters via pre-commit
	@pre-commit run -v --all-files

.PHONY: test
test: ## Run all tests with race detector
	@go test -timeout=5m -race ./...

.PHONY: coverage
coverage: ## Run tests with coverage report
	@mkdir -p coverage
	@go test -timeout=5m -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage/coverage.html
	@go tool cover -func coverage.out | tail -1

##@ Documentation
.PHONY: gen-man
gen-man: ## Generate man pages to ./man/
	@mkdir -p man
	@go run . docs man -o ./man/

.PHONY: gen-markdown
gen-markdown: ## Generate markdown CLI docs to ./docs/cli/
	@mkdir -p docs/cli
	@go run . docs markdown -o ./docs/cli/
