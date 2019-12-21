package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	promconfig "gitlab.com/gitlab-org/gitaly/internal/config/prometheus"
)

// RegisterReplicationLatency creates and registers a prometheus histogram
// to observe replication latency times
func RegisterReplicationLatency(conf promconfig.Config) Histogram {
	replicationLatency := prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "gitaly",
			Subsystem: "praefect",
			Name:      "replication_latency",
			Buckets:   conf.GRPCLatencyBuckets,
		},
	)

	prometheus.MustRegister(replicationLatency)
	return replicationLatency
}

// RegisterReplicationJobsInFlight creates and registers a gauge
// to track the size of the replication queue
func RegisterReplicationJobsInFlight() Gauge {
	replicationJobsInFlight := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "gitaly",
			Subsystem: "praefect",
			Name:      "replication_jobs",
		},
	)
	prometheus.MustRegister(replicationJobsInFlight)
	return replicationJobsInFlight
}

// Gauge is a subset of a prometheus Gauge
type Gauge interface {
	Inc()
	Dec()
}

// Histogram is a subset of a prometheus Histogram
type Histogram interface {
	Observe(float64)
}
