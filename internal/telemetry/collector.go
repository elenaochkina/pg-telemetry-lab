package telemetry

import (
	"context"
	"time"
)

// Collector collects telemetry metrics from PostgreSQL.
type Collector interface {
	// CollectReplicationLag collects replication lag metrics.
	CollectReplicationLag(ctx context.Context) (*ReplicationMetrics, error)

	// StartPolling starts continuous metric collection with given interval.
	StartPolling(ctx context.Context, interval time.Duration, writer MetricsWriter) error
}

// MetricsWriter writes collected metrics to output.
// Implementations: JSON (stdout/file), Grafana Cloud, Prometheus
type MetricsWriter interface {
	Write(metrics *ReplicationMetrics) error
	Close() error
}
