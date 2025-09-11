package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rhobs_synthetics_api_http_requests_total",
			Help: "The total number of HTTP requests handled by the API.",
		},
		[]string{"code", "method"},
	)

	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rhobs_synthetics_api_http_request_duration_seconds",
			Help:    "A histogram of the request latencies.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)

	httpRequestsInFlight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "rhobs_synthetics_api_http_requests_in_flight",
			Help: "The number of HTTP requests currently being processed.",
		},
	)

	probestoreRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rhobs_synthetics_api_probestore_request_duration_seconds",
			Help:    "The latency of operations against the active probe store.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	probestoreErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rhobs_synthetics_api_probestore_errors_total",
			Help: "The total number of errors encountered when interacting with the probe store.",
		},
		[]string{"operation"},
	)

	probesTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rhobs_synthetics_api_probes_total",
			Help: "The total number of probe configs.",
		},
		[]string{"state", "private"},
	)
)

func RegisterMetrics() {
	prometheus.MustRegister(
		httpRequestsTotal,
		httpRequestDuration,
		httpRequestsInFlight,
		probestoreRequestDuration,
		probestoreErrorsTotal,
		probesTotal,
	)
}

func RecordProbestoreRequest(operation string, start time.Time) {
	probestoreRequestDuration.WithLabelValues(operation).Observe(time.Since(start).Seconds())
}

func RecordProbestoreError(operation string) {
	probestoreErrorsTotal.WithLabelValues(operation).Inc()
}

func SetProbesTotal(state, private string, count int) {
	probesTotal.WithLabelValues(state, private).Set(float64(count))
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func NewResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := NewResponseWriter(w)

		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		next.ServeHTTP(rw, r)

		duration := time.Since(start)
		statusCode := strconv.Itoa(rw.statusCode)

		httpRequestsTotal.WithLabelValues(statusCode, r.Method).Inc()
		httpRequestDuration.WithLabelValues(r.Method).Observe(duration.Seconds())
	})
}

func Handler() http.Handler {
	return promhttp.Handler()
}
