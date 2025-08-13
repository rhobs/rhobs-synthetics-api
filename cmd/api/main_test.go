package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRouter(t *testing.T) {
	// Create a simple test handler for the validated API
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test api"))
	})

	// Create a minimal swagger spec for testing
	swagger := &openapi3.T{
		OpenAPI: "3.0.0",
		Info: &openapi3.Info{
			Title:   "Test API",
			Version: "1.0.0",
		},
	}

	// Test with nil clientset (local storage mode)
	router := createRouter(testHandler, nil, swagger)
	assert.NotNil(t, router)

	// Test health endpoints
	testCases := []struct {
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{"/livez", http.StatusOK, "ok"},
		{"/readyz", http.StatusOK, "ok"},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code)
			assert.Equal(t, tc.expectedBody, w.Body.String())
		})
	}

	// Test docs endpoint
	t.Run("/docs", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/docs", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	})

	// Test OpenAPI spec endpoint
	t.Run("/api/v1/openapi.json", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/openapi.json", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	})
}

func TestCreateProbeStore(t *testing.T) {
	// Save original viper values
	originalEngine := viper.GetString("database_engine")
	originalNamespace := viper.GetString("namespace")
	originalDataDir := viper.GetString("data_dir")

	// Reset viper after test
	defer func() {
		viper.Set("database_engine", originalEngine)
		viper.Set("namespace", originalNamespace)
		viper.Set("data_dir", originalDataDir)
	}()

	t.Run("local storage", func(t *testing.T) {
		viper.Set("database_engine", "local")
		viper.Set("data_dir", "")

		store, clientset, err := createProbeStore()

		require.NoError(t, err)
		assert.NotNil(t, store)
		assert.Nil(t, clientset)
	})

	t.Run("local storage with custom data dir", func(t *testing.T) {
		viper.Set("database_engine", "local")
		viper.Set("data_dir", "/tmp/test-probes")

		store, clientset, err := createProbeStore()

		require.NoError(t, err)
		assert.NotNil(t, store)
		assert.Nil(t, clientset)
	})

	t.Run("unsupported database engine", func(t *testing.T) {
		viper.Set("database_engine", "unsupported")

		store, clientset, err := createProbeStore()

		require.Error(t, err)
		assert.Nil(t, store)
		assert.Nil(t, clientset)
		assert.Contains(t, err.Error(), "unsupported database engine")
	})
}