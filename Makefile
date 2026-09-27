.PHONY: help build run run-no-email test test-v test-coverage fmt vet lint vulncheck gitleaks release-snapshot setup-hooks clean act-ci act-test-no-email act-test act-main

export PATH := $(HOME)/go/bin:$(PATH)

# Default target
.DEFAULT_GOAL := help

help: ## Show this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

## Local Workflow Simulation (act / gh act)
act-ci: ## Run CI workflow via gh act
	gh act -W .github/workflows/ci.yml --use-gitignore=false

act-test-no-email: ## Run test-no-email workflow via gh act with artifacts saved to ./artifacts
	@mkdir -p ./artifacts
	gh act -W .github/workflows/test-no-email.yml --secret-file .env --use-gitignore=false --artifact-server-path ./artifacts

act-test: ## Run test workflow via gh act with artifacts saved to ./artifacts
	@mkdir -p ./artifacts
	gh act -W .github/workflows/test.yml --secret-file .env --use-gitignore=false --artifact-server-path ./artifacts

act-main: ## Run main workflow via gh act with artifacts saved to ./artifacts
	@mkdir -p ./artifacts
	gh act -W .github/workflows/main.yml --secret-file .env --use-gitignore=false --artifact-server-path ./artifacts

## Development & Building
build: ## Build the Go application binary
	@mkdir -p bin
	go build -o bin/zotero-arxiv-daily go/cmd/zotero-arxiv-daily/main.go

run: ## Run application locally using config directory
	go run go/cmd/zotero-arxiv-daily/main.go -config config

run-no-email: ## Run application locally with NO_EMAIL=true
	NO_EMAIL=true SAVE_EMAIL_PATH=email_output.html go run go/cmd/zotero-arxiv-daily/main.go -config config

## Testing & Quality
test: ## Run all unit tests
	go test ./...

test-v: ## Run all unit tests with verbose output
	go test -v ./...

test-coverage: ## Run all unit tests and generate coverage report
	@mkdir -p coverage
	go test -coverprofile=coverage/coverage.out ./...
	go tool cover -html=coverage/coverage.out -o coverage/coverage.html
	@echo "Coverage report generated at coverage/coverage.html"

fmt: ## Format Go source code with gofmt
	find go -name "*.go" -exec gofmt -s -w {} +

vet: ## Run go vet on codebase
	go vet ./...

lint: ## Run golangci-lint on codebase
	golangci-lint run ./...

vulncheck: ## Run govulncheck vulnerability scanner
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

gitleaks: ## Run gitleaks secret detection locally
	gitleaks detect --config .gitleaks.toml --verbose

## Release & Packaging
release-snapshot: ## Test GoReleaser build without publishing
	goreleaser release --snapshot --clean

## Git & Tooling
setup-hooks: ## Configure git hooks path to .githooks
	git config core.hooksPath .githooks
	chmod +x .githooks/*
	@echo "Git hooks configured to .githooks"

clean: ## Clean built binaries, test artifacts, and coverage reports
	rm -rf bin/ coverage/ ./artifacts/ dist/ email_output.html
