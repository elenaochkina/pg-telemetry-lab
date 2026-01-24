package writer

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"

	"github.com/elenaochkina/pg-telemetry-lab/internal/telemetry"
)

// PrometheusWriter writes telemetry metrics to Prometheus using Remote Write protocol.
type PrometheusWriter struct {
	endpoint string
	username string
	password string
	client   *http.Client
	labels   map[string]string // Global labels added to all metrics
}

// NewPrometheusWriter creates a new Prometheus Remote Write client.
// endpoint: Prometheus remote write endpoint (e.g., Grafana Cloud URL)
// username: Basic auth username (for Grafana Cloud, this is the instance ID)
// password: Basic auth password (for Grafana Cloud, this is the API key)
// labels: Global labels to add to all metrics (e.g., cluster, environment)
func NewPrometheusWriter(endpoint, username, password string, labels map[string]string) *PrometheusWriter {
	return &PrometheusWriter{
		endpoint: endpoint,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		labels: labels,
	}
}

// Write converts ReplicationMetrics to Prometheus format and sends via Remote Write.
func (w *PrometheusWriter) Write(metrics *telemetry.ReplicationMetrics) error {
	// Convert to Prometheus time series
	timeseries := w.convertToPrometheusSamples(metrics)

	// Create remote write request
	req := &prompb.WriteRequest{
		Timeseries: timeseries,
	}

	// Serialize to protobuf
	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshaling protobuf: %w", err)
	}

	// Compress with snappy
	compressed := snappy.Encode(nil, data)

	// Send via HTTP POST
	return w.sendRemoteWrite(compressed)
}

// convertToPrometheusSamples converts ReplicationMetrics to Prometheus TimeSeries.
func (w *PrometheusWriter) convertToPrometheusSamples(metrics *telemetry.ReplicationMetrics) []prompb.TimeSeries {
	timestamp := metrics.Timestamp.UnixMilli()
	var timeseries []prompb.TimeSeries

	// Primary LSN metric
	timeseries = append(timeseries, w.createTimeSeries(
		"pg_primary_lsn_bytes",
		float64(metrics.Primary.CurrentLSN),
		timestamp,
		map[string]string{
			"host": metrics.Primary.Host,
		},
	))

	// Replication connection metrics (from primary's view)
	for _, conn := range metrics.Primary.Connections {
		labels := map[string]string{
			"application_name": conn.ApplicationName,
			"client_addr":      conn.ClientAddr,
			"state":            conn.State,
			"sync_state":       conn.SyncState,
		}

		timeseries = append(timeseries, w.createTimeSeries(
			"pg_replication_lag_bytes",
			float64(conn.LagBytes),
			timestamp,
			labels,
		))
	}

	// Replica metrics (from replica's view)
	for _, replica := range metrics.Replicas {
		labels := map[string]string{
			"replica_host":      replica.Host,
			"subscription_name": replica.SubscriptionName,
		}

		// Lag in bytes
		timeseries = append(timeseries, w.createTimeSeries(
			"pg_replica_lag_bytes",
			float64(replica.LagBytes),
			timestamp,
			labels,
		))

		// Lag in seconds
		timeseries = append(timeseries, w.createTimeSeries(
			"pg_replica_lag_seconds",
			replica.LagSeconds,
			timestamp,
			labels,
		))
	}

	return timeseries
}

// createTimeSeries creates a Prometheus TimeSeries with given name, value, timestamp, and labels.
func (w *PrometheusWriter) createTimeSeries(name string, value float64, timestamp int64, labels map[string]string) prompb.TimeSeries {
	// Merge with global labels
	allLabels := make(map[string]string)
	for k, v := range w.labels {
		allLabels[k] = v
	}
	for k, v := range labels {
		allLabels[k] = v
	}

	// Convert to Prometheus labels
	promLabels := []prompb.Label{
		{Name: "__name__", Value: name},
	}
	for k, v := range allLabels {
		promLabels = append(promLabels, prompb.Label{
			Name:  k,
			Value: v,
		})
	}

	return prompb.TimeSeries{
		Labels: promLabels,
		Samples: []prompb.Sample{
			{
				Value:     value,
				Timestamp: timestamp,
			},
		},
	}
}

// sendRemoteWrite sends compressed protobuf data to Prometheus Remote Write endpoint.
func (w *PrometheusWriter) sendRemoteWrite(data []byte) error {
	req, err := http.NewRequest("POST", w.endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	// Set required headers for Prometheus Remote Write
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("User-Agent", "pg-telemetry-lab/1.0")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	// Basic authentication (required by Grafana Cloud)
	req.SetBasicAuth(w.username, w.password)

	// Send request
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Close cleans up resources (no-op for PrometheusWriter).
func (w *PrometheusWriter) Close() error {
	return nil
}
