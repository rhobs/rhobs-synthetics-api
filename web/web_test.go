package web

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSwaggerHTML(t *testing.T) {
	// Test that SwaggerHTML is not empty
	assert.NotEmpty(t, SwaggerHTML, "SwaggerHTML should not be empty")

	// Test that it contains expected HTML content
	assert.Contains(t, string(SwaggerHTML), "html", "SwaggerHTML should contain HTML content")

	// Test that the embedded content is valid (basic check)
	assert.True(t, len(SwaggerHTML) > 100, "SwaggerHTML should contain substantial content")
}