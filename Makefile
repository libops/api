.PHONY: help proto proto-clean api provider install-provider test clean all

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

##@ Proto Generation

proto: ## Generate Go code and OpenAPI spec from .proto files
	@echo "Generating protobuf code..."
	@cd proto && buf generate
	@echo "Generating OpenAPI spec..."
	@cd proto && buf generate --template buf.gen.openapi.yaml
	@echo "Injecting OAuth scopes into OpenAPI spec..."
	@python3 proto/inject-oauth-scopes.py
	@echo "Splitting OpenAPI spec into customer and admin versions..."
	@python3 proto/split-openapi.py
	@echo "Protobuf code and OpenAPI spec generated successfully"

sqlc: ## Generate database code with sqlc
	@echo "Generating sqlc database code..."
	sqlc generate
	@echo "SQLC database code generated successfully"

generate: proto sqlc ## Generate all code (proto, OpenAPI, sqlc, provider schemas)
	@echo "✅ All code generation complete!"

proto-clean: ## Clean generated proto files
	@echo "Cleaning generated proto files..."
	@find proto -name "*.pb.go" -type f -delete
	@find proto -name "*_connect.go" -type f -delete
	@rm -f openapi/openapi.yaml
	@echo "Cleaned generated proto files"

sqlc-clean: ## Clean generated sqlc files
	@echo "Cleaning generated sqlc files..."
	@rm -rf internal/db/*.go
	@echo "Cleaned generated sqlc files"

proto-lint: ## Lint proto files
	@echo "Linting proto files..."
	@cd proto && buf lint

proto-breaking: ## Check for breaking changes in proto files
	@echo "Checking for breaking changes..."
	@cd proto && buf breaking --against '.git#branch=main'

fmt: ## Format all Go code
	@echo "Formatting Go code..."
	@gofmt -w **/*.go

lint: generate ## Lint all Go code (requires golangci-lint)
	@echo "Linting api..."
	@golangci-lint run
	@go mod tidy
	@git diff --quiet go.mod go.sum


test: ## Run all tests
	@echo "Running tests..."
	@go test -v -race ./internal/...

##@ Integration Tests

integration-test: ## Run integration tests with Docker Compose
	@echo "Running integration tests..."
	@cd ci && ./run-tests.sh --clean --build
	@echo "Integration tests complete"

integration-test-logs: ## View integration test logs
	@cd ci && docker compose logs -f

integration-test-clean: ## Clean up integration test environment
	@echo "Cleaning integration test environment..."
	@cd ci && docker compose down -v
	@echo "Integration test environment cleaned"

integration-test-db: ## Access integration test database
	@cd ci && docker compose exec mariadb mysql -u libops -plibops-test-pass libops

##@ Cleanup

clean: ## Clean all build artifacts
	@echo "Cleaning build artifacts..."
	@find . -name "*.pb.go" -type f -delete
	@echo "Cleaned all build artifacts"

clean-generated: proto-clean sqlc-clean ## Clean all generated code (proto + sqlc)
	@echo "All generated code cleaned"

clean-all: clean clean-generated ## Clean everything including generated code
	@echo "All artifacts and generated code cleaned"

##@ Dependencies

deps: ## Download all dependencies
	@echo "Downloading dependencies..."
	@cd api && go mod download
	@cd provider && go mod download
	@echo "Dependencies downloaded"

deps-update: ## Update all dependencies
	@echo "Updating dependencies..."
	@cd api && go get -u ./... && go mod tidy
	@cd provider && go get -u ./... && go mod tidy
	@echo "Dependencies updated"

##@ Tools

install-tools: ## Install required development tools
	@echo "Installing development tools..."
	@go install github.com/bufbuild/buf/cmd/buf@latest
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
	@go install github.com/google/gnostic/cmd/protoc-gen-openapi@v0.7.0
	@go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
