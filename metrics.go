package main

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/agentkourier/demo-checkout/client"
)

// metrics holds the service's Prometheus collectors.
type metrics struct {
	requests         *prometheus.CounterVec
	duration         *prometheus.HistogramVec
	paymentsRequests *prometheus.CounterVec
}

func newMetrics(reg prometheus.Registerer) *metrics {
	m := &metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "HTTP requests served, by handler and status code.",
		}, []string{"handler", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Time to serve HTTP requests, by handler.",
			Buckets: prometheus.DefBuckets,
		}, []string{"handler"}),
		paymentsRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "payments_client_requests_total",
			Help: "Authorize calls to payments, by outcome: ok, http_error or decode_error.",
		}, []string{"outcome"}),
	}
	// Export every outcome from the start so rate() works before the first error.
	for _, outcome := range []string{client.OutcomeOK, client.OutcomeHTTPError, client.OutcomeDecodeError} {
		m.paymentsRequests.WithLabelValues(outcome)
	}
	reg.MustRegister(
		m.requests,
		m.duration,
		m.paymentsRequests,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// instrument records request metrics and writes one log line per request.
// It must run inside the otelhttp handler so the request context carries the
// server span, and outside the ServeMux so r.Pattern is set once next returns.
func instrument(logger *slog.Logger, m *metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)
		elapsed := time.Since(start)

		handler := route(r)
		m.requests.WithLabelValues(handler, strconv.Itoa(rec.code)).Inc()
		m.duration.WithLabelValues(handler).Observe(elapsed.Seconds())

		if isProbe(r.URL.Path) {
			return
		}
		level := slog.LevelInfo
		if rec.code >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		logger.LogAttrs(r.Context(), level, "request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rec.code),
			slog.Float64("duration_ms", float64(elapsed.Microseconds())/1000),
		)
	})
}

// route returns the matched path pattern without its method, or "unmatched".
func route(r *http.Request) string {
	if r.Pattern == "" {
		return "unmatched"
	}
	if _, path, ok := strings.Cut(r.Pattern, " "); ok {
		return path
	}
	return r.Pattern
}

func isProbe(path string) bool {
	return path == "/healthz" || path == "/readyz" || path == "/metrics"
}

type statusRecorder struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.code = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
