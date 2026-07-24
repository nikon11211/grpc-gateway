package server

import "github.com/prometheus/client_golang/prometheus"

var (
	histogramRequestDurationBuckets = []float64{
		0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 50,
		100, 200, 500, 1000, 2000, 5000, 10000,
	}
)

type MetricsRegistry struct {
	SuccessCounter  *prometheus.CounterVec
	ErrorCounter    *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	registry        *prometheus.Registry
}

func NewMetricsRegistry(namespace string) *MetricsRegistry {
	reg := prometheus.NewRegistry()

	successCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "grpc_requests_success_total",
			Help:      "Total number of successful gRPC requests",
		},
		[]string{"method"},
	)

	errorCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "grpc_requests_error_total",
			Help:      "Total number of failed gRPC requests",
		},
		[]string{"method", "error_code"},
	)

	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "grpc_request_duration_seconds",
			Help:      "Histogram of latencies for gRPC requests",
			Buckets:   histogramRequestDurationBuckets,
		},
		[]string{"method"},
	)

	reg.MustRegister(successCounter, errorCounter, requestDuration)

	return &MetricsRegistry{
		SuccessCounter:  successCounter,
		ErrorCounter:    errorCounter,
		RequestDuration: requestDuration,
		registry:        reg,
	}
}

func (m *MetricsRegistry) GetRegistry() *prometheus.Registry {
	return m.registry
}
