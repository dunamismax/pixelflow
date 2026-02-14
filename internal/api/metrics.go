package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metrics struct {
	registry          *prometheus.Registry
	requestTotal      *prometheus.CounterVec
	requestDuration   *prometheus.HistogramVec
	rateLimitRejected *prometheus.CounterVec
	queueEnqueued     *prometheus.CounterVec
}

func newMetrics() *metrics {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &metrics{
		registry: registry,
		requestTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pixelflow_api_requests_total",
			Help: "Total HTTP requests handled by the API.",
		}, []string{"method", "route", "status"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pixelflow_api_request_duration_seconds",
			Help:    "API request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
		rateLimitRejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pixelflow_api_rate_limit_rejections_total",
			Help: "Total API requests rejected by rate limiting.",
		}, []string{"route"}),
		queueEnqueued: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pixelflow_queue_jobs_enqueued_total",
			Help: "Total jobs enqueued to the processing queue.",
		}, []string{"queue"}),
	}
	registry.MustRegister(
		m.requestTotal,
		m.requestDuration,
		m.rateLimitRejected,
		m.queueEnqueued,
	)
	return m
}

func (m *metrics) metricsHandler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *metrics) withHTTPMetrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)

		route := routeLabel(r.URL.Path)
		status := statusLabel(recorder.status)

		m.requestTotal.WithLabelValues(r.Method, route, status).Inc()
		m.requestDuration.WithLabelValues(r.Method, route, status).Observe(time.Since(start).Seconds())
	})
}

func statusLabel(status int) string {
	return strconv.Itoa(status)
}

func routeLabel(path string) string {
	switch {
	case strings.HasPrefix(path, "/v1/jobs/") && strings.HasSuffix(path, "/start"):
		return "/v1/jobs/{id}/start"
	case strings.HasPrefix(path, "/v1/jobs"):
		return "/v1/jobs"
	case strings.HasPrefix(path, "/healthz"):
		return "/healthz"
	case strings.HasPrefix(path, "/metrics"):
		return "/metrics"
	default:
		return path
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
