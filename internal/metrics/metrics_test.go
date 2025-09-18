package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddleware(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(httpRequestsTotal)

	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	expectedCounter := `
		# HELP rhobs_synthetics_api_http_requests_total The total number of HTTP requests handled by the API.
		# TYPE rhobs_synthetics_api_http_requests_total counter
		rhobs_synthetics_api_http_requests_total{code="200",method="GET"} 1
	`
	err := testutil.CollectAndCompare(httpRequestsTotal, strings.NewReader(expectedCounter))
	assert.NoError(t, err)
}

func TestRecordProbestoreMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(probestoreRequestDuration)
	reg.MustRegister(probestoreErrorsTotal)

	RecordProbestoreRequest("get_probe", time.Now())
	RecordProbestoreError("get_probe")

	expectedErrors := `
		# HELP rhobs_synthetics_api_probestore_errors_total The total number of errors encountered when interacting with the probe store.
		# TYPE rhobs_synthetics_api_probestore_errors_total counter
		rhobs_synthetics_api_probestore_errors_total{operation="get_probe"} 1
	`
	err := testutil.CollectAndCompare(probestoreErrorsTotal, strings.NewReader(expectedErrors))
	assert.NoError(t, err)

	count := testutil.CollectAndCount(probestoreRequestDuration)
	assert.Equal(t, 1, count)
}

func TestSetProbesTotal(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(probesTotal)

	SetProbesTotal("active", "true", 5)

	expectedGauge := `
		# HELP rhobs_synthetics_api_probes_total The total number of probe configs.
		# TYPE rhobs_synthetics_api_probes_total gauge
		rhobs_synthetics_api_probes_total{private="true",state="active"} 5
	`
	err := testutil.CollectAndCompare(probesTotal, strings.NewReader(expectedGauge))
	assert.NoError(t, err)
}

func TestHandler(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(httpRequestsTotal)

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	body, err := io.ReadAll(rr.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "rhobs_synthetics_api_http_requests_total")
}
