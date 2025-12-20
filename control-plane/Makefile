.PHONY: help proto proto-clean api provider install-provider test clean all

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

sqlc: ## Generate database code with sqlc
	@echo "Generating sqlc database code..."
	sqlc generate
	@echo "SQLC database code generated successfully"

sqlc-clean: ## Clean generated sqlc files
	@echo "Cleaning generated sqlc files..."
	@rm -rf internal/db/*.go
	@echo "Cleaned generated sqlc files"

fmt: ## Format all Go code
	@echo "Formatting Go code..."
	@find . -type f -name "*.go" -exec gofmt -w {} \;

lint: ## Lint all Go code (requires golangci-lint)
	@echo "Linting api..."
	@golangci-lint run
	@go mod tidy
	@git diff --quiet go.mod go.sum


test: ## Run all tests
	@echo "Running tests..."
	@go test -v -race ./...

install-tools: ## Install required development tools
	@echo "Installing development tools..."
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0
