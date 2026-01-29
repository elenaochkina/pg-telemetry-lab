package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/elenaochkina/pg-telemetry-lab/internal/db"
	"github.com/elenaochkina/pg-telemetry-lab/internal/topology"
	"github.com/jackc/pgx/v5"
)

// PostgresCollector collects telemetry metrics from any PostgreSQL cluster.
// It is provider-agnostic and works with Docker, AWS RDS, GCP Cloud SQL, or on-prem PostgreSQL.
type PostgresCollector struct {
	primary  topology.PGTarget
	replicas []topology.PGTarget
	password string
}

// NewPostgresCollector creates a new provider-agnostic PostgreSQL telemetry collector.
// It accepts connection targets for primary and replicas, making it reusable across all infrastructure providers.
func NewPostgresCollector(primary topology.PGTarget, replicas []topology.PGTarget, password string) *PostgresCollector {
	return &PostgresCollector{
		primary:  primary,
		replicas: replicas,
		password: password,
	}
}

// CollectReplicationLag collects replication lag metrics from primary and replicas.
func (c *PostgresCollector) CollectReplicationLag(ctx context.Context) (*ReplicationMetrics, error) {
	metrics := &ReplicationMetrics{
		Timestamp: time.Now(),
		Replicas:  []ReplicaMetrics{},
	}

	// Collect metrics from primary
	primaryMetrics, err := c.collectPrimaryMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("collecting primary metrics: %w", err)
	}
	metrics.Primary = *primaryMetrics

	// Collect metrics from each replica
	for _, target := range c.replicas {
		replicaMetrics, err := c.collectReplicaMetrics(ctx, target)
		if err != nil {
			// Log error but continue with other replicas
			fmt.Printf("Warning: failed to collect metrics from %s: %v\n", target.Label, err)
			continue
		}
		metrics.Replicas = append(metrics.Replicas, *replicaMetrics)
	}

	return metrics, nil
}

// collectPrimaryMetrics collects metrics from the primary database.
func (c *PostgresCollector) collectPrimaryMetrics(ctx context.Context) (*PrimaryMetrics, error) {
	conn, err := db.Connect(ctx, c.primary, c.password)
	if err != nil {
		return nil, fmt.Errorf("connecting to primary: %w", err)
	}
	defer conn.Close(ctx)

	metrics := &PrimaryMetrics{
		Host:        c.primary.Addr(),
		Connections: []ReplicationSlot{},
	}

	// Get current LSN
	var currentLSN string
	err = conn.QueryRow(ctx, "SELECT pg_current_wal_lsn()").Scan(&currentLSN)
	if err != nil {
		return nil, fmt.Errorf("querying current LSN: %w", err)
	}

	// Convert LSN to bytes
	var lsnBytes int64
	err = conn.QueryRow(ctx, "SELECT pg_wal_lsn_diff($1, '0/0')", currentLSN).Scan(&lsnBytes)
	if err != nil {
		return nil, fmt.Errorf("converting LSN to bytes: %w", err)
	}
	metrics.CurrentLSN = lsnBytes

	// Query pg_stat_replication for connected replicas
	// Note: Using pg_current_wal_lsn() instead of sent_lsn to capture total lag
	// including any sender delays on the primary.
	query := `
		SELECT
			application_name,
			client_addr::text,
			state,
			sent_lsn::text,
			write_lsn::text,
			flush_lsn::text,
			replay_lsn::text,
			COALESCE(pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn), 0) AS lag_bytes,
			sync_state
		FROM pg_stat_replication
	`

	rows, err := conn.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying pg_stat_replication: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var slot ReplicationSlot
		err := rows.Scan(
			&slot.ApplicationName,
			&slot.ClientAddr,
			&slot.State,
			&slot.SentLSN,
			&slot.WriteLSN,
			&slot.FlushLSN,
			&slot.ReplayLSN,
			&slot.LagBytes,
			&slot.SyncState,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning replication slot: %w", err)
		}
		metrics.Connections = append(metrics.Connections, slot)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating replication slots: %w", err)
	}

	return metrics, nil
}

// collectReplicaMetrics collects metrics from a replica database.
func (c *PostgresCollector) collectReplicaMetrics(ctx context.Context, target topology.PGTarget) (*ReplicaMetrics, error) {
	conn, err := db.Connect(ctx, target, c.password)
	if err != nil {
		return nil, fmt.Errorf("connecting to replica: %w", err)
	}
	defer conn.Close(ctx)

	metrics := &ReplicaMetrics{
		Host: target.Addr(),
	}

	// Query pg_stat_subscription for subscription metrics
	query := `
		SELECT
			subname,
			received_lsn::text,
			latest_end_lsn::text,
			COALESCE(pg_wal_lsn_diff(latest_end_lsn, received_lsn), 0) AS lag_bytes,
			last_msg_send_time,
			last_msg_receipt_time,
			latest_end_time
		FROM pg_stat_subscription
		LIMIT 1
	`

	var latestEndTime *time.Time
	err = conn.QueryRow(ctx, query).Scan(
		&metrics.SubscriptionName,
		&metrics.ReceivedLSN,
		&metrics.LatestEndLSN,
		&metrics.LagBytes,
		&metrics.LastMsgSendTime,
		&metrics.LastMsgRecvTime,
		&latestEndTime,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			// No subscription found, this is okay (replica might not be set up yet)
			return metrics, nil
		}
		return nil, fmt.Errorf("querying pg_stat_subscription: %w", err)
	}

	// Calculate lag in milliseconds
	if latestEndTime != nil {
		metrics.LagMilliseconds = float64(time.Since(*latestEndTime).Milliseconds())
	}

	return metrics, nil
}

// StartPolling starts continuous metric collection with given interval.
func (c *PostgresCollector) StartPolling(ctx context.Context, interval time.Duration, writer MetricsWriter) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Collect immediately on start
	metrics, err := c.CollectReplicationLag(ctx)
	if err != nil {
		fmt.Printf("Error collecting metrics: %v\n", err)
	} else {
		if err := writer.Write(metrics); err != nil {
			fmt.Printf("Error writing metrics: %v\n", err)
		} else {
			fmt.Printf("✓ Collected metrics at %s\n", metrics.Timestamp.Format("15:04:05"))
		}
	}

	// Then poll at regular intervals
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Check if context is still valid before collecting
			if ctx.Err() != nil {
				return ctx.Err()
			}

			metrics, err := c.CollectReplicationLag(ctx)
			if err != nil {
				// If context was cancelled/expired, return gracefully
				if ctx.Err() != nil {
					return ctx.Err()
				}
				// Otherwise, log error and continue
				fmt.Printf("Error collecting metrics: %v\n", err)
				continue
			}
			if err := writer.Write(metrics); err != nil {
				fmt.Printf("Error writing metrics: %v\n", err)
			} else {
				fmt.Printf("✓ Collected metrics at %s\n", metrics.Timestamp.Format("15:04:05"))
			}
		}
	}
}
