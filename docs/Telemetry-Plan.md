# Telemetry Collection Plan: Replication Lag

This document outlines the implementation plan for collecting replication lag telemetry in pg-telemetry-lab.

---

## Overview

**Goal:** Collect replication lag metrics from PostgreSQL logical replication using polling-based monitoring.

**Initial Scope:**
- Replication lag metrics (LSN-based)
- JSON output format
- Two execution modes:
  1. Integrated with benchmark runs
  2. Standalone CLI command

---

## Key Requirements Summary

| Requirement | Implementation |
|-------------|----------------|
| **Metrics** | Replication lag (long integer), LSN movement on Primary (long integer) |
| **Collection Method** | Periodic polling with telemetry agent |
| **Deployment** | Separate Go program running in dedicated container |
| **Architecture** | Generic telemetry package based on PG system catalogs, provider-specific collector startup/deployment |
| **Output** | Grafana Cloud (free tier), with metric connector interface to support future backends |
| **Runtime** | Runs continuously while primary is running, shows spikes during benchmark runs |
| **Initial Format** | JSON for debugging, Prometheus remote write for Grafana Cloud |
| **CLI Commands** | `telemetryctl telemetry collect` (standalone), automatic start with cluster provisioning |

---

## Metrics to Collect

### Replication Lag Metrics

From **Primary** (`pg_stat_replication`):
```sql
SELECT
    application_name,
    client_addr,
    state,
    sent_lsn,
    write_lsn,
    flush_lsn,
    replay_lsn,
    pg_wal_lsn_diff(sent_lsn, replay_lsn) AS lag_bytes,
    sync_state,
    sync_priority
FROM pg_stat_replication;
```

From **Replica** (`pg_stat_wal_receiver`):
```sql
SELECT
    status,
    receive_start_lsn,
    receive_start_tli,
    received_lsn,
    received_tli,
    last_msg_send_time,
    last_msg_receipt_time,
    latest_end_lsn,
    latest_end_time
FROM pg_stat_wal_receiver;
```

From **Replica** (`pg_stat_subscription`):
```sql
SELECT
    subname,
    pid,
    received_lsn,
    latest_end_lsn,
    pg_wal_lsn_diff(latest_end_lsn, received_lsn) AS lag_bytes,
    last_msg_send_time,
    last_msg_receipt_time,
    latest_end_time
FROM pg_stat_subscription;
```

### Calculated Metrics

- **Lag in bytes**: `pg_wal_lsn_diff(primary_lsn, replica_lsn)`
- **Lag in time**: `EXTRACT(EPOCH FROM (now() - latest_end_time))`
- **Replication throughput**: Bytes replicated per second
- **Catch-up status**: Whether replica is behind or caught up

---

## Architecture

### High-Level Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     Docker Compose Cluster                   │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐      ┌──────────────┐  ┌──────────────┐  │
│  │   Primary    │─────▶│  Replica 1   │  │  Replica 2   │  │
│  │  (pgbench)   │      │              │  │              │  │
│  │  :5432       │      │  :5540       │  │  :5541       │  │
│  └──────┬───────┘      └──────┬───────┘  └──────┬───────┘  │
│         │                     │                 │           │
│         │                     │                 │           │
│         └─────────────────────┴─────────────────┘           │
│                               │                             │
│                               │ SQL Queries                 │
│                               │ (pg_stat_*)                 │
│                               │                             │
│                    ┌──────────▼──────────┐                  │
│                    │  Telemetry Agent    │                  │
│                    │  (Separate Container)│                  │
│                    │  - Polls every 5s   │                  │
│                    │  - Collects metrics │                  │
│                    └──────────┬──────────┘                  │
│                               │                             │
└───────────────────────────────┼─────────────────────────────┘
                                │
                                │ Push Metrics
                                │ (Prometheus Remote Write)
                                ▼
                    ┌───────────────────────┐
                    │   Grafana Cloud       │
                    │   - Free Tier         │
                    │   - Dashboards        │
                    │   - Alerts            │
                    └───────────────────────┘
```

**Data Flow:**
1. **Telemetry Agent** polls PostgreSQL system catalogs every 5 seconds:
   - Primary: `pg_stat_replication` (LSN sent, replication slots)
   - Replicas: `pg_stat_subscription` (LSN received, lag)
2. **Metrics Collected**:
   - Replication lag in bytes (long integer)
   - LSN movement on primary (long integer)
   - Lag in seconds (calculated)
3. **Metrics Written**:
   - JSON format to stdout/file (for debugging)
   - Prometheus format to Grafana Cloud (for visualization)
4. **Grafana Cloud**:
   - Stores metrics with 14-day retention
   - Visualizes replication lag over time
   - Shows spikes during benchmark runs
   - Sends alerts if lag exceeds thresholds

### Package Structure

```
internal/telemetry/
├── collector.go         # Collector interface
├── replication.go       # Replication lag collector
├── metrics.go           # Metric structs
├── json.go              # JSON formatter
└── writer.go            # MetricsWriter interface and implementations

internal/telemetry/writer/
├── json_writer.go       # JSON/JSONL writer
├── grafana_writer.go    # Grafana Cloud push writer
└── prometheus_writer.go # Prometheus remote write (future)

internal/provider/dockerpg/telemetry/
└── collector.go         # Docker-specific collector implementation

cmd/telemetry-agent/
└── main.go              # Standalone telemetry agent
```

### Design Pattern

Follow the same provider-agnostic pattern as benchmark and replication:

1. **Interface**: `internal/telemetry/collector.go` (provider-agnostic)
2. **Implementation**: `internal/provider/dockerpg/telemetry/` (Docker-specific)
3. **Factory**: `cmd/factory.go` creates appropriate collector
4. **Standalone Agent**: `cmd/telemetry-agent/` runs as separate container

### Collector Interface

```go
// internal/telemetry/collector.go
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
```

### Metrics Structs

```go
// internal/telemetry/metrics.go
package telemetry

import "time"

// ReplicationMetrics contains all replication lag metrics.
type ReplicationMetrics struct {
    Timestamp    time.Time         `json:"timestamp"`
    Primary      PrimaryMetrics    `json:"primary"`
    Replicas     []ReplicaMetrics  `json:"replicas"`
}

// PrimaryMetrics contains metrics from the primary database.
type PrimaryMetrics struct {
    Host         string                `json:"host"`
    Connections  []ReplicationSlot     `json:"connections"`
}

// ReplicationSlot represents a single replication connection.
type ReplicationSlot struct {
    ApplicationName string  `json:"application_name"`
    ClientAddr      string  `json:"client_addr"`
    State           string  `json:"state"`
    SentLSN         string  `json:"sent_lsn"`
    WriteLSN        string  `json:"write_lsn"`
    FlushLSN        string  `json:"flush_lsn"`
    ReplayLSN       string  `json:"replay_lsn"`
    LagBytes        int64   `json:"lag_bytes"`
    SyncState       string  `json:"sync_state"`
}

// ReplicaMetrics contains metrics from a replica database.
type ReplicaMetrics struct {
    Host            string    `json:"host"`
    SubscriptionName string   `json:"subscription_name"`
    ReceivedLSN     string    `json:"received_lsn"`
    LatestEndLSN    string    `json:"latest_end_lsn"`
    LagBytes        int64     `json:"lag_bytes"`
    LagSeconds      float64   `json:"lag_seconds"`
    LastMsgSendTime time.Time `json:"last_msg_send_time"`
    LastMsgRecvTime time.Time `json:"last_msg_recv_time"`
}
```

### MetricsWriter Implementations

#### JSONWriter

Writes metrics to stdout or file in JSON/JSONL format:

```go
// internal/telemetry/writer/json_writer.go
type JSONWriter struct {
    output io.Writer
    pretty bool
}

func NewJSONWriter(output io.Writer, pretty bool) *JSONWriter
func (w *JSONWriter) Write(metrics *ReplicationMetrics) error
func (w *JSONWriter) Close() error
```

#### GrafanaWriter

Pushes metrics to Grafana Cloud using their metrics API:

```go
// internal/telemetry/writer/grafana_writer.go
type GrafanaWriter struct {
    endpoint string
    apiKey   string
    client   *http.Client
}

func NewGrafanaWriter(endpoint, apiKey string) *GrafanaWriter
func (w *GrafanaWriter) Write(metrics *ReplicationMetrics) error
func (w *GrafanaWriter) Close() error
```

**Configuration:**
- Grafana Cloud endpoint: `https://prometheus-prod-XX-prod-XX-XXXX.grafana.net/api/prom/push`
- API Key: From Grafana Cloud account
- Store in `config.yaml`:
  ```yaml
  telemetry:
    writer: grafana  # or "json", "prometheus"
    grafana:
      endpoint: "https://..."
      api_key: "glc_..."
  ```

---

## Deployment Models

### Model 1: Standalone Telemetry Agent (Separate Container)

**Purpose:** Run telemetry collection continuously while the primary database is running.

**Architecture:**
- New Go program: `cmd/telemetry-agent/main.go`
- Runs in separate Docker container alongside primary and replicas
- Connects to primary and replicas to collect metrics
- Pushes metrics to Grafana Cloud or writes to file
- Runs continuously with configurable polling interval

**Docker Compose Integration:**

```yaml
services:
  pgbench-primary:
    image: postgres:17
    # ... existing config ...

  pgbench-replica-1:
    image: postgres:17
    # ... existing config ...

  telemetry-agent:
    build:
      context: .
      dockerfile: Dockerfile.telemetry
    environment:
      - TELEMETRY_INTERVAL=5s
      - TELEMETRY_WRITER=grafana
      - GRAFANA_ENDPOINT=${GRAFANA_ENDPOINT}
      - GRAFANA_API_KEY=${GRAFANA_API_KEY}
      - PRIMARY_HOST=pgbench-primary
      - PRIMARY_PORT=5432
      - REPLICA_HOSTS=pgbench-replica-1:5432,pgbench-replica-2:5432
    depends_on:
      - pgbench-primary
      - pgbench-replica-1
    networks:
      - pgbench-net
```

**Dockerfile:**

```dockerfile
# Dockerfile.telemetry
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o telemetry-agent cmd/telemetry-agent/main.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/telemetry-agent /telemetry-agent
ENTRYPOINT ["/telemetry-agent"]
```

**Agent Program:**

```go
// cmd/telemetry-agent/main.go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "time"

    "github.com/yourusername/pg-telemetry-lab/internal/telemetry"
    "github.com/yourusername/pg-telemetry-lab/internal/telemetry/writer"
)

func main() {
    // Load config from environment
    interval := parseDuration(os.Getenv("TELEMETRY_INTERVAL"), 5*time.Second)
    writerType := getEnv("TELEMETRY_WRITER", "json")

    // Create metrics writer
    var metricsWriter telemetry.MetricsWriter
    switch writerType {
    case "grafana":
        metricsWriter = writer.NewGrafanaWriter(
            os.Getenv("GRAFANA_ENDPOINT"),
            os.Getenv("GRAFANA_API_KEY"),
        )
    case "json":
        metricsWriter = writer.NewJSONWriter(os.Stdout, false)
    }
    defer metricsWriter.Close()

    // Create collector
    collector := createCollector()

    // Start polling
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle shutdown gracefully
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt)
    go func() {
        <-sigChan
        log.Println("Shutting down telemetry agent...")
        cancel()
    }()

    log.Printf("Starting telemetry collection (interval=%s, writer=%s)\n", interval, writerType)
    if err := collector.StartPolling(ctx, interval, metricsWriter); err != nil {
        log.Fatalf("Telemetry collection failed: %v", err)
    }
}
```

**Lifecycle:**
1. Start with cluster: `./telemetryctl provision local` (automatically starts telemetry container)
2. Runs continuously, collecting metrics every 5 seconds
3. Pushes to Grafana Cloud in real-time
4. During benchmarks, you'll see spikes in replication lag
5. Stop with cluster: `./telemetryctl teardown local`

### Model 2: Integrated CLI Command

**Purpose:** Collect telemetry on-demand or during specific benchmark runs.

This is the original design where telemetry is invoked via CLI commands like:
- `./telemetryctl telemetry collect`
- `./telemetryctl benchmark local --collect-telemetry`

---

## CLI Integration

### New Command: `telemetry collect`

```bash
# Collect metrics once and print to stdout
./telemetryctl telemetry collect

# Collect metrics continuously with 5s interval
./telemetryctl telemetry collect --interval 5s

# Collect for specific duration
./telemetryctl telemetry collect --interval 5s --duration 1m

# Write to file
./telemetryctl telemetry collect --interval 5s --output metrics.jsonl
```

### CLI Structure

```go
// cmd/cli.go - Add new command
case "telemetry":
    return handleTelemetry(args[1:])

// cmd/telemetryHandlers.go - New file
func handleTelemetry(args []string) error {
    if len(args) == 0 {
        return fmt.Errorf("telemetry subcommand required")
    }

    switch args[0] {
    case "collect":
        return handleTelemetryCollect(args[1:])
    default:
        return fmt.Errorf("unknown telemetry subcommand: %s", args[0])
    }
}

func handleTelemetryCollect(args []string) error {
    // Parse flags: --interval, --duration, --output
    // Load config
    // Create collector via factory
    // Start collection
}
```

### Benchmark Integration

Update `cmd/handlers.go`:

```go
func handleBenchmark(args []string) error {
    // ... existing code ...

    // If --collect-telemetry flag is set:
    collector, err := createTelemetryCollector(cfg)
    if err != nil {
        return err
    }

    // Start telemetry collection in background
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go collector.StartPolling(ctx, 5*time.Second, jsonWriter)

    // Run benchmark
    runner.Run(opts)

    // Stop telemetry collection
    cancel()
}
```

---

## Implementation Phases

### Phase 1: Core Telemetry Infrastructure

**Goal:** Basic telemetry collection from PostgreSQL system catalogs.

**Tasks:**
1. Create `internal/telemetry/` package
   - Define `Collector` interface
   - Define metric structs (`ReplicationMetrics`, etc.)
   - Create JSON formatter (`JSONWriter`)

2. Create `internal/provider/dockerpg/telemetry/` package
   - Implement `DockerCollector`
   - Connect to primary and replicas
   - Query `pg_stat_replication` (primary)
   - Query `pg_stat_subscription` (replicas)

3. Add factory function
   - `cmd/factory.go`: `createTelemetryCollector(cfg)`

**Acceptance Criteria:**
- Can create collector via factory
- Can collect metrics once from primary
- Can collect metrics once from replicas
- Metrics are returned as struct

---

### Phase 2: JSON Output & CLI Command

**Goal:** Standalone telemetry collection command with JSON output.

**Tasks:**
1. Implement JSON formatter
   - `internal/telemetry/json.go`: `JSONWriter`
   - Pretty-print JSON
   - Support JSONL (JSON Lines) for streaming

2. Create CLI command
   - `cmd/telemetryHandlers.go`
   - `telemetry collect` subcommand
   - Parse flags: `--interval`, `--duration`, `--output`

3. Implement single-shot collection
   - Collect once
   - Print JSON to stdout

**Acceptance Criteria:**
```bash
./telemetryctl telemetry collect
# Outputs:
{
  "timestamp": "2026-01-21T14:30:00Z",
  "primary": {
    "host": "localhost:5432",
    "connections": [...]
  },
  "replicas": [...]
}
```

---

### Phase 3: Continuous Polling

**Goal:** Continuous metric collection with configurable interval.

**Tasks:**
1. Implement polling logic
   - `collector.StartPolling(ctx, interval, writer)`
   - Use `time.Ticker` for regular intervals
   - Respect context cancellation

2. Add CLI flags
   - `--interval 5s` (default: 5 seconds)
   - `--duration 1m` (default: run forever)
   - `--output metrics.jsonl` (default: stdout)

3. Implement file output
   - Write JSONL format (one JSON object per line)
   - Append mode
   - Flush after each write

**Acceptance Criteria:**
```bash
# Run for 30 seconds, collect every 5s
./telemetryctl telemetry collect --interval 5s --duration 30s

# Run continuously, write to file
./telemetryctl telemetry collect --interval 5s --output metrics.jsonl
```

---

### Phase 4: Benchmark Integration

**Goal:** Automatically collect telemetry during benchmark runs.

**Tasks:**
1. Add benchmark flag
   - `--collect-telemetry` (enable telemetry during benchmark)
   - `--telemetry-interval 5s`
   - `--telemetry-output benchmark-metrics.jsonl`

2. Integrate with benchmark command
   - Start telemetry collector in background goroutine
   - Run benchmark
   - Stop collector when benchmark completes
   - Merge telemetry data with benchmark results

3. Correlate metrics with benchmark
   - Add benchmark phase to metrics (init, run, complete)
   - Timestamp alignment

**Acceptance Criteria:**
```bash
./telemetryctl benchmark local \
  --duration 60 \
  --clients 20 \
  --collect-telemetry \
  --telemetry-output benchmark-metrics.jsonl

# Produces:
# - Benchmark TPS output (stdout)
# - Replication lag metrics (benchmark-metrics.jsonl)
```

---

### Phase 5: Grafana Cloud Integration

**Goal:** Push metrics to Grafana Cloud for visualization and alerting.

**Tasks:**
1. Create Grafana Cloud writer
   - `internal/telemetry/writer/grafana_writer.go`
   - Implement Prometheus remote write protocol
   - Convert metrics to Prometheus format
   - Handle authentication with API key

2. Add configuration
   - Update `config.yaml` to support Grafana Cloud settings
   - Support environment variables for credentials
   - Validate endpoint and API key

3. Test Grafana Cloud push
   - Create free Grafana Cloud account
   - Configure API key and endpoint
   - Test metric push
   - Verify metrics appear in Grafana Cloud

4. Add CLI support
   - `--writer grafana` flag for telemetry command
   - Read Grafana config from environment or config file

**Configuration Example:**

```yaml
# config.yaml
telemetry:
  writer: grafana  # Options: json, grafana, prometheus
  interval: 5s
  grafana:
    endpoint: "https://prometheus-prod-XX-prod-XX-XXXX.grafana.net/api/prom/push"
    api_key: "glc_XXXXXXXXXXXXXXXX"
    labels:
      cluster: "pg-telemetry-lab"
      environment: "dev"
```

**Acceptance Criteria:**
```bash
# Set Grafana credentials
export GRAFANA_ENDPOINT="https://prometheus-prod-XX.grafana.net/api/prom/push"
export GRAFANA_API_KEY="glc_XXXXXXXXXXXXXXXX"

# Collect and push to Grafana Cloud
./telemetryctl telemetry collect --writer grafana --interval 5s

# Verify metrics in Grafana Cloud dashboard
```

**Metrics Format (Prometheus):**
```
pg_replication_lag_bytes{application_name="pgbench_sub_1",cluster="pg-telemetry-lab"} 1024
pg_replication_lag_seconds{subscription_name="pgbench_sub_1",replica="localhost:5540"} 0.002
pg_lsn_sent{cluster="pg-telemetry-lab"} 50331648
```

---

### Phase 6: Standalone Telemetry Agent (Separate Container)

**Goal:** Deploy telemetry collection as a standalone service in a separate container.

**Tasks:**
1. Create telemetry agent program
   - `cmd/telemetry-agent/main.go`
   - Standalone Go program that runs continuously
   - Loads config from environment variables
   - Handles graceful shutdown

2. Create Dockerfile
   - `Dockerfile.telemetry`
   - Multi-stage build for minimal image size
   - Include CA certificates for HTTPS

3. Update Docker Compose
   - Add `telemetry-agent` service to `docker-compose.yml`
   - Configure environment variables
   - Set up dependencies on primary and replicas
   - Ensure agent starts after databases are ready

4. Update provision command
   - Automatically start telemetry agent with cluster
   - Pass Grafana Cloud credentials if configured
   - Handle agent lifecycle with cluster

5. Logging and monitoring
   - Add structured logging to agent
   - Log collection status, errors, and metrics
   - Handle connection failures gracefully

**File Structure:**
```
cmd/telemetry-agent/
├── main.go              # Standalone agent entry point
└── config.go            # Agent configuration from env vars

Dockerfile.telemetry     # Agent container image
docker-compose.yml       # Updated with telemetry service
```

**Acceptance Criteria:**
```bash
# Provision cluster with telemetry agent
./telemetryctl provision local

# Verify telemetry agent is running
docker ps | grep telemetry-agent

# Check telemetry agent logs
docker logs pgbench-telemetry-agent

# Run benchmark and verify metrics are collected
./telemetryctl benchmark local --duration 60 --clients 20

# Check Grafana Cloud dashboard for replication lag spikes during benchmark

# Teardown cluster (stops telemetry agent too)
./telemetryctl teardown local
```

**Agent Behavior:**
- Starts automatically with cluster provisioning
- Runs continuously, polling every 5 seconds (configurable)
- Pushes metrics to Grafana Cloud in real-time
- Shows replication lag spikes during benchmark runs
- Stops gracefully when cluster is torn down
- Handles database connection failures with retry logic
- Logs all activity to stdout for Docker logs

---

## Example Output

### Single Collection

```json
{
  "timestamp": "2026-01-21T14:30:00Z",
  "primary": {
    "host": "localhost:5432",
    "connections": [
      {
        "application_name": "pgbench_sub_1",
        "client_addr": "172.20.0.3",
        "state": "streaming",
        "sent_lsn": "0/3000000",
        "write_lsn": "0/3000000",
        "flush_lsn": "0/3000000",
        "replay_lsn": "0/2FFFFFF",
        "lag_bytes": 1,
        "sync_state": "async"
      }
    ]
  },
  "replicas": [
    {
      "host": "localhost:5540",
      "subscription_name": "pgbench_sub_1",
      "received_lsn": "0/3000000",
      "latest_end_lsn": "0/3000000",
      "lag_bytes": 0,
      "lag_seconds": 0.001,
      "last_msg_send_time": "2026-01-21T14:29:59Z",
      "last_msg_recv_time": "2026-01-21T14:30:00Z"
    },
    {
      "host": "localhost:5541",
      "subscription_name": "pgbench_sub_2",
      "received_lsn": "0/2FFFFFE",
      "latest_end_lsn": "0/3000000",
      "lag_bytes": 2,
      "lag_seconds": 0.002,
      "last_msg_send_time": "2026-01-21T14:29:59Z",
      "last_msg_recv_time": "2026-01-21T14:29:59Z"
    }
  ]
}
```

### Continuous Collection (JSONL)

```jsonl
{"timestamp":"2026-01-21T14:30:00Z","primary":{...},"replicas":[...]}
{"timestamp":"2026-01-21T14:30:05Z","primary":{...},"replicas":[...]}
{"timestamp":"2026-01-21T14:30:10Z","primary":{...},"replicas":[...]}
```

---

## Future Enhancements

After completing Phase 6, consider:

1. **Additional Metrics**
   - WAL metrics (`pg_stat_wal`)
   - Transaction stats (`pg_stat_database`)
   - Connection stats
   - Table/index stats
   - Query performance metrics

2. **Additional Output Formats**
   - Prometheus scrape endpoint (pull-based)
   - CSV format for analysis
   - InfluxDB line protocol
   - OpenTelemetry format

3. **Grafana Dashboards**
   - Pre-built dashboard templates
   - Replication lag visualization
   - WAL pressure monitoring
   - Performance correlation charts
   - Automated dashboard provisioning

4. **Analysis Tools**
   - CLI command to analyze collected metrics
   - Generate reports (average lag, max lag, percentiles)
   - Historical trend analysis
   - Anomaly detection

5. **Alerting**
   - Define replication lag thresholds
   - Alert when lag exceeds limits
   - Grafana Cloud alert rules
   - Webhook integrations (Slack, PagerDuty)

6. **Optimization**
   - Connection pooling for database queries
   - Batch queries to reduce overhead
   - Adaptive polling intervals
   - Metric aggregation for high-frequency data

---

## Implementation Order and Dependencies

```
Phase 1: Core Infrastructure
    │
    ├─▶ Phase 2: JSON Output & CLI
    │       │
    │       └─▶ Phase 3: Continuous Polling
    │               │
    │               ├─▶ Phase 4: Benchmark Integration
    │               │
    │               └─▶ Phase 5: Grafana Cloud Integration
    │                       │
    │                       └─▶ Phase 6: Standalone Agent
```

**Dependencies:**
- Phase 2-6 all require Phase 1 (core infrastructure)
- Phase 3 requires Phase 2 (CLI command framework)
- Phase 4 and Phase 5 both require Phase 3 (polling mechanism)
- Phase 6 requires Phase 5 (Grafana Cloud integration)

**Recommended Implementation Order:**
1. **Phase 1** → **Phase 2** → **Phase 3**: Build core functionality with JSON output
2. **Phase 4**: Add benchmark integration (test with local workloads)
3. **Phase 5**: Add Grafana Cloud integration (visualize metrics)
4. **Phase 6**: Deploy as standalone agent (production-ready deployment)

**Parallel Development:**
- Phase 4 and Phase 5 can be developed in parallel after Phase 3 is complete
- Both can be tested independently before Phase 6

---

## Grafana Cloud Setup

### Free Tier Account

Grafana Cloud offers a free tier that includes:
- 10,000 series for Prometheus metrics
- 14-day retention
- 3 users
- Pre-built dashboards and alerting

**Setup Steps:**

1. **Create Account**
   - Go to https://grafana.com/auth/sign-up/create-user
   - Sign up with email
   - Select "Free" plan

2. **Get API Credentials**
   - Navigate to "Connections" → "Add new connection"
   - Select "Hosted Prometheus metrics"
   - Copy the endpoint URL: `https://prometheus-prod-XX-prod-XX-XXXX.grafana.net/api/prom/push`
   - Generate API key with "MetricsPublisher" role
   - Save credentials securely

3. **Configure Telemetry Agent**
   ```bash
   # Add to .env file (DO NOT commit)
   GRAFANA_ENDPOINT=https://prometheus-prod-XX-prod-XX-XXXX.grafana.net/api/prom/push
   GRAFANA_API_KEY=glc_XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
   ```

4. **Create Dashboard**
   - Navigate to "Dashboards" → "New" → "New Dashboard"
   - Add panel for replication lag:
     ```promql
     pg_replication_lag_bytes{cluster="pg-telemetry-lab"}
     ```
   - Add panel for LSN movement:
     ```promql
     rate(pg_lsn_sent[1m])
     ```
   - Save dashboard

5. **Set Up Alerts** (Optional)
   - Create alert rule for high replication lag
   - Threshold: `pg_replication_lag_bytes > 10485760` (10MB)
   - Notification channel: Email, Slack, etc.

### Metrics Pushed to Grafana Cloud

| Metric Name | Type | Description | Labels |
|------------|------|-------------|--------|
| `pg_replication_lag_bytes` | Gauge | Replication lag in bytes | `application_name`, `cluster` |
| `pg_replication_lag_seconds` | Gauge | Replication lag in seconds | `subscription_name`, `replica`, `cluster` |
| `pg_lsn_sent` | Counter | LSN sent from primary (as integer) | `cluster` |
| `pg_lsn_replay` | Counter | LSN replayed on replica (as integer) | `replica`, `cluster` |
| `pg_replication_state` | Gauge | Replication connection state (1=streaming, 0=other) | `application_name`, `state`, `cluster` |

---

## Testing Strategy

### Unit Tests
- Test metric collection from mock database
- Test JSON formatting
- Test polling logic with fake ticker

### Integration Tests
- Start Docker cluster
- Collect metrics
- Verify metrics are accurate
- Test during benchmark runs

### Manual Testing
```bash
# Start cluster
./telemetryctl provision local
./telemetryctl replication setup

# Generate load
./telemetryctl benchmark local --duration 60 --clients 20 &

# Collect telemetry
./telemetryctl telemetry collect --interval 1s --duration 60s --output test.jsonl

# Analyze
cat test.jsonl | jq '.replicas[0].lag_bytes'
```

---

## Success Criteria

### Phase 1-4 (Core Functionality)
- [ ] Can collect replication lag from primary and replicas
- [ ] Metrics output in JSON format
- [ ] Standalone CLI command works: `telemetryctl telemetry collect`
- [ ] Continuous polling works with configurable interval
- [ ] Can write to file in JSONL format
- [ ] Telemetry collection integrates with benchmark runs

### Phase 5 (Grafana Cloud Integration)
- [ ] Can push metrics to Grafana Cloud
- [ ] Metrics appear in Grafana Cloud dashboard
- [ ] Prometheus format conversion works correctly
- [ ] Configuration via environment variables and config file

### Phase 6 (Standalone Agent)
- [ ] Telemetry agent runs as separate container
- [ ] Agent starts automatically with cluster provisioning
- [ ] Agent runs continuously and collects metrics
- [ ] Agent pushes to Grafana Cloud in real-time
- [ ] Replication lag spikes visible during benchmark runs
- [ ] Agent stops gracefully with cluster teardown

### Overall
- [ ] Documentation updated
- [ ] Basic tests pass
- [ ] End-to-end testing with real workloads
- [ ] Grafana Cloud dashboard shows replication metrics

---

## Next Steps

1. ✅ Review and approve this plan
2. Start with Phase 1: Core Telemetry Infrastructure
   - Implement collector interface
   - Create Docker-specific implementation
   - Define metric structs
3. Proceed through phases 2-6 sequentially
4. Test end-to-end with benchmark workloads
5. Set up Grafana Cloud dashboard for visualization
6. Deploy as standalone agent in production-like environment
