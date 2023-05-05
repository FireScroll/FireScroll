package internal

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	Metric_HTTPLatenciesMicro = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_latencies_micro",
		Help:    "Full HTTP request processing latencies. Includes remote lookups for gets.",
		Buckets: []float64{100, 200, 300, 400, 500, 750, 1000, 1250, 1500, 2000, 2500, 3000, 4000, 5000, 6000, 10_000, 15_000},
	}, []string{"operation"})
	Metric_LocalPartitionLatenciesMicro = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "partition_operation_latencies_micro",
		Help:    "Latencies for partition-level operations in microseconds",
		Buckets: []float64{10, 15, 25, 50, 75, 100, 200, 300, 400, 500, 750, 1000, 1250, 1500, 2000, 2500, 3000},
	}, []string{"operation"})
	Metric_Partitions = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "partitions",
		Help: "The number of partitions on this node",
	})
)

func registerMetrics() {

}
