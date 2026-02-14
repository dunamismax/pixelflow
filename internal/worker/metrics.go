package worker

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metrics struct {
	registry             *prometheus.Registry
	jobsTotal            *prometheus.CounterVec
	jobDuration          *prometheus.HistogramVec
	activeJobs           prometheus.Gauge
	pipelineOutputsTotal prometheus.Counter
	pixelsProcessedTotal prometheus.Counter
	bytesSavedTotal      prometheus.Counter
	computeTimeMSTotal   prometheus.Counter
}

func newMetrics() *metrics {
	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &metrics{
		registry: registry,
		jobsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "pixelflow_worker_jobs_total",
			Help: "Total worker jobs by source type and final status.",
		}, []string{"source_type", "status"}),
		jobDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pixelflow_worker_job_duration_seconds",
			Help:    "Total processing duration for each worker job.",
			Buckets: prometheus.DefBuckets,
		}, []string{"source_type", "status"}),
		activeJobs: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "pixelflow_worker_active_jobs",
			Help: "Current number of active processing jobs in the worker.",
		}),
		pipelineOutputsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pixelflow_worker_pipeline_outputs_total",
			Help: "Total transformed outputs emitted by the worker.",
		}),
		pixelsProcessedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pixelflow_usage_pixels_processed_total",
			Help: "Total pixels processed across all successful jobs.",
		}),
		bytesSavedTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pixelflow_usage_bytes_saved_total",
			Help: "Total bytes saved across all successful jobs.",
		}),
		computeTimeMSTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "pixelflow_usage_compute_time_ms_total",
			Help: "Total compute time in milliseconds across successful jobs.",
		}),
	}

	registry.MustRegister(
		m.jobsTotal,
		m.jobDuration,
		m.activeJobs,
		m.pipelineOutputsTotal,
		m.pixelsProcessedTotal,
		m.bytesSavedTotal,
		m.computeTimeMSTotal,
	)
	return m
}

func (m *metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
