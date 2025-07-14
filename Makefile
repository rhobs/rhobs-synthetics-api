# Makefile for building the rhobs-synthetics-api binary
OAPI_CODEGEN_VERSION=v2.4.1

# The name of the binary to be built
BINARY_NAME=rhobs-synthetics-api

# The main package of the application
MAIN_PACKAGE=./cmd/api/main.go

.PHONY: all build clean run help lint lint-ci tidy generate ensure-oapi-codegen

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

lint:
	$(GOBIN)/golangci-lint run ./...

lint-ci:
	$(GOBIN)/golangci-lint run ./... --output.text.path=stdout --timeout=5m

test:
	go test -cover ./...

tidy:
	go mod tidy

# Clean up build artifacts
clean:
	@echo "Cleaning up..."
	@go clean
	@rm -f $(BINARY_NAME)
	@echo "Cleanup complete."

