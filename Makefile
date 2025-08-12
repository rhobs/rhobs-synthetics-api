# Makefile for building the rhobs-synthetics-api binary
OAPI_CODEGEN_VERSION=v2.4.1
# Replace 'your-quay-namespace' with your actual Quay.io namespace.
IMAGE_URL ?= quay.io/app-sre/rhobs/rhobs-synthetics-api
# Image tag, defaults to 'latest'
TAG ?= latest
# Namespace where the resources will be deployed
NAMESPACE ?= rhobs
# Konflux manifests directory
KONFLUX_DIR := konflux
# The name of the binary to be built
BINARY_NAME=rhobs-synthetics-api
# The main package of the application
MAIN_PACKAGE=./cmd/api/main.go
# podman vs. docker
CONTAINER_ENGINE ?= podman
TESTOPTS ?= -cover

.PHONY: all build clean run help lint lint-fix lint-ci go-mod-tidy go-mod-download generate ensure-oapi-codegen docker-build docker-push

all: build

# Build the Go binary
build:
	@echo "Building $(BINARY_NAME)..."
	@go build -o $(BINARY_NAME) $(MAIN_PACKAGE)
	@echo "$(BINARY_NAME) built successfully."

# Ensures oapi-codegen is installed locally.
# It checks if the binary exists in GOPATH/bin, and if not, it installs it.
ensure-oapi-codegen:
	@echo "Ensuring oapi-codegen is installed locally..."
	@ls $(GOPATH)/bin/oapi-codegen 1>/dev/null 2>&1 || (echo "oapi-codegen not found. Installing version ${OAPI_CODEGEN_VERSION}..." && go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@${OAPI_CODEGEN_VERSION})

# Generates Go code from OpenAPI specifications.
generate: ensure-oapi-codegen
	@echo "Generating OpenAPI code locally..."
	@mkdir -p pkg/client # Ensure pkg/client directory exists
	$(GOENV) go generate -v ./...

GOLANGCI_LINT_VERSION ?= v2.0.2
GOLANGCI_LINT_BIN := $(shell go env GOPATH)/bin/golangci-lint

lint: $(GOLANGCI_LINT_BIN)
	$(GOLANGCI_LINT_BIN) run ./...

lint-ci: $(GOLANGCI_LINT_BIN)
	$(GOLANGCI_LINT_BIN) run ./... --output.text.path=stdout --timeout=5m

lint-fix: $(GOLANGCI_LINT_BIN)
	$(GOLANGCI_LINT_BIN) run --fix ./...

$(GOLANGCI_LINT_BIN):
	@echo "Checking for golangci-lint..."
	@if [ ! -f "$@" ]; then \
		echo "golangci-lint not found. Installing $(GOLANGCI_LINT_VERSION)..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(dir $@) $(GOLANGCI_LINT_VERSION); \
	else \
		echo "golangci-lint already installed."; \
	fi

test: go-mod-download
	go test $(TESTOPTS) ./...

.PHONY: coverage
coverage:
	hack/codecov.sh

go-mod-tidy:
	go mod tidy

go-mod-download:
	go mod download

# Build the Docker image
docker-build:
	@echo "Building Docker image for linux/amd64: $(IMAGE_URL):$(TAG)"
	$(CONTAINER_ENGINE) build --platform linux/amd64 -t $(IMAGE_URL):$(TAG) .

# Push the Docker image to the registry
docker-push:
	@echo "Pushing Docker image: $(IMAGE_URL):$(TAG)"
	$(CONTAINER_ENGINE) push $(IMAGE_URL):$(TAG)

# Clean up build artifacts
clean:
	@echo "Cleaning up..."
	@go clean
	@rm -f $(BINARY_NAME)
	@$(CONTAINER_ENGINE) rmi -f -i $(IMAGE_URL):$(TAG)
	@echo "Cleanup complete."
