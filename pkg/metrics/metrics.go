package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "sentinel_proxy_requests_total",
		Help: "The total number of processed requests",
	}, []string{"method", "status", "backend"})

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "sentinel_proxy_request_duration_seconds",
		Help:    "The duration of processed requests",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "backend"})

	BackendHealth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sentinel_proxy_backend_health",
		Help: "Current health status of backends (1 = healthy, 0 = unhealthy)",
	}, []string{"url"})

	BackendIntegrity = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sentinel_proxy_backend_integrity_score",
		Help: "Integrity score of backends (0-100)",
	}, []string{"url"})

	BackendBlockNumber = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "sentinel_proxy_backend_block_number",
		Help: "Latest block number of backends",
	}, []string{"url"})
)

func Register() {
	// promauto registers automatically to the DefaultRegisterer
}

// RecordRequest increments the request counter
func RecordRequest(method, status, backend string) {
	RequestTotal.WithLabelValues(method, status, backend).Inc()
}

// ObserveRequestDuration observes the request duration
func ObserveRequestDuration(method, backend string, duration float64) {
	RequestDuration.WithLabelValues(method, backend).Observe(duration)
}

// SetBackendHealth sets the health gauge for a backend
func SetBackendHealth(url string, healthy bool) {
	val := 0.0
	if healthy {
		val = 1.0
	}
	BackendHealth.WithLabelValues(url).Set(val)
}

// SetBackendIntegrity sets the integrity score gauge
func SetBackendIntegrity(url string, score int) {
	BackendIntegrity.WithLabelValues(url).Set(float64(score))
}

// SetBackendBlockNumber sets the block number gauge
func SetBackendBlockNumber(url string, blockNum int) {
	BackendBlockNumber.WithLabelValues(url).Set(float64(blockNum))
}
