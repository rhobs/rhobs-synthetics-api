unexport GOFLAGS

GOOS?=linux
GOARCH?=amd64
GOENV=GOOS=${GOOS} GOARCH=${GOARCH} CGO_ENABLED=0 GOFLAGS=
GOBUILDFLAGS=-gcflags="all=-trimpath=${GOPATH}" -asmflags="all=-trimpath=${GOPATH}"

OAPI_CODEGEN_VERSION=v2.4.1

# Default target
all: generate

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

