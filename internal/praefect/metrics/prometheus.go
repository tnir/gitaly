package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	promconfig "gitlab.com/gitlab-org/gitaly/internal/config/prometheus"
	"gitlab.com/gitlab-org/gitaly/internal/praefect/config"
	"gitlab.com/gitlab-org/gitaly/internal/prometheus/metrics"
)

// RegisterReplicationDelay creates and registers a prometheus histogram
// to observe replication delay times
func RegisterReplicationDelay(conf promconfig.Config) (metrics.HistogramVec, error) {
	replicationDelay := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gitaly",
			Subsystem: "praefect",
			Name:      "replication_delay",
			Buckets:   conf.GRPCLatencyBuckets,
		},
		[]string{"type"},
	)

	return replicationDelay, prometheus.Register(replicationDelay)
}

// RegisterReplicationLatency creates and registers a prometheus histogram
// to observe replication latency times
func RegisterReplicationLatency(conf promconfig.Config) (metrics.HistogramVec, error) {
	replicationLatency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gitaly",
			Subsystem: "praefect",
			Name:      "replication_latency",
			Buckets:   conf.GRPCLatencyBuckets,
		},
		[]string{"type"},
	)

	return replicationLatency, prometheus.Register(replicationLatency)
}

// RegisterNodeLatency creates and registers a prometheus histogram to
// observe internal node latency
func RegisterNodeLatency(conf promconfig.Config) (metrics.HistogramVec, error) {
	nodeLatency := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "gitaly",
			Subsystem: "praefect",
			Name:      "node_latency",
			Buckets:   conf.GRPCLatencyBuckets,
		}, []string{"gitaly_storage"},
	)

	return nodeLatency, prometheus.Register(nodeLatency)
}

// RegisterTransactionVoters creates and registers a Prometheus counter to gauge
// the number of voters per transaction.
func RegisterTransactionVoters(conf config.Config) (metrics.HistogramVec, error) {
	maxVoters := 0
	for _, storage := range conf.VirtualStorages {
		if len(storage.Nodes) > maxVoters {
			maxVoters = len(storage.Nodes)
		}
	}

	transactionVoters := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gitaly_praefect_voters_per_transaction_total",
			Help:    "The number of voters a given transaction was created with",
			Buckets: prometheus.LinearBuckets(0, 1, maxVoters),
		},
		[]string{"virtual_storage"},
	)
	return transactionVoters, prometheus.Register(transactionVoters)
}

var MethodTypeCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "gitaly",
		Subsystem: "praefect",
		Name:      "method_types",
	}, []string{"method_type"},
)

var PrimaryGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "gitaly",
		Subsystem: "praefect",
		Name:      "primaries",
	}, []string{"virtual_storage", "gitaly_storage"},
)

var NodeLastHealthcheckGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Namespace: "gitaly",
		Subsystem: "praefect",
		Name:      "node_last_healthcheck_up",
	}, []string{"gitaly_storage"},
)

var ChecksumMismatchCounter = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "gitaly",
		Subsystem: "praefect",
		Name:      "checksum_mismatch_total",
	}, []string{"target", "source"},
)

// ReadDistribution counts how many read operations was routed to each storage.
var ReadDistribution = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "gitaly",
		Subsystem: "praefect",
		Name:      "read_distribution",
		Help:      "Counts read operations directed to the storages",
	},
	[]string{"virtual_storage", "storage"},
)

func init() {
	prometheus.MustRegister(
		MethodTypeCounter,
		PrimaryGauge,
		ChecksumMismatchCounter,
		NodeLastHealthcheckGauge,
		ReadDistribution,
	)
}
