// Package codegen contains tooling for OpenAPI code generation.
// This package is not part of the runtime application.
package codegen

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config cfg.yaml ../../api/v1/openapi.yaml
