unexport GOFLAGS

GOOS?=linux
GOARCH?=amd64
GOENV=GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=0 GOFLAGS=
GOBUILDFLAGS=-gcflags="all=-trimpath=${GOPATH}" -asmflags="all=-trimpath=${GOPATH}"

RUN_IN_CONTAINER_CMD:=$(CONTAINER_ENGINE) run --platform linux/amd64 --rm -v $(shell pwd):/app -w=/app backplane-api-builder /bin/bash -c

OAPI_CODEGEN_VERSION=v2.4.1

# Default target
all: generate

# Ensures oapi-codegen is installed locally.
# It checks if the binary exists in GOPATH/bin, and if not, it installs it.
ensure-oapi-codegen:
	@echo "Ensuring oapi-codegen is installed locally..."
	@ls $(GOPATH)/bin/oapi-codegen 1>/dev/null 2>&1 || (echo "oapi-codegen not found. Installing version ${OAPI_CODEGEN_VERSION}..." && go install github.com/deepmap/oapi-codegen/cmd/oapi-codegen@${OAPI_CODEGEN_VERSION})

# Generates Go code from OpenAPI specifications.
# This target now runs entirely locally.
generate: ensure-oapi-codegen
	@echo "Generating OpenAPI code locally..."
	@mkdir -p pkg/client # Ensure pkg/client directory exists
	$(GOENV) go generate -v ./...
