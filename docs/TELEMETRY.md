# Telemetry Guide

Complete guide to PostgreSQL replication metrics collection, Grafana Cloud integration, and visualization.

---

## Overview

The telemetry system collects real-time replication lag metrics from PostgreSQL using a **provider-agnostic design**. It works with:

- Docker containers (local development)
- AWS RDS (cloud databases)
- GCP Cloud SQL (cloud databases)
- On-premise PostgreSQL installations
- Any PostgreSQL 10+ with logical replication

**Key insight:** Telemetry collection is independent of infrastructure provisioning. You configure connection details via environment variables, not YAML config files.

---

## Architecture

### Provider-Agnostic Design

```
┌──────────────────────────────────────┐
│ Environment Variables                │
│  • PG_PRIMARY_HOST                   │
│  • PG_REPLICA_HOSTS                  │
│  • GRAFANA_CLOUD_ENDPOINT            │
└──────────────┬───────────────────────┘
               │
┌──────────────▼───────────────────────┐
│ Telemetry Collector                  │
│  (internal/telemetry/)               │
│  • Provider-agnostic                 │
│  • Works with any PostgreSQL         │
└──────────────┬───────────────────────┘
               │
┌──────────────▼───────────────────────┐
│ Writers                              │
│  • JSON/JSONL (local file)           │
│  • Prometheus Remote Write (Grafana) │
└──────────────────────────────────────┘
```

**Why environment variables instead of config?**

1. **Decoupling:** Provisioning (Docker/AWS) is separate from monitoring
2. **Flexibility:** Same code works with RDS, Cloud SQL, on-prem
3. **12-Factor App:** Environment-based configuration is a best practice
4. **Simplicity:** No need for provider-specific collector implementations

---

## Metrics Collected

### Primary Metrics

| Metric | Type | Description | Source |
|--------|------|-------------|--------|
| `pg_primary_lsn_bytes` | Gauge | Current WAL LSN position (bytes) | `pg_current_wal_lsn()` on primary |

**Labels:**
- `cluster` - Cluster name (e.g., "pg-telemetry-lab")
- `environment` - Environment (e.g., "dev", "prod")
- `provider` - Infrastructure provider (e.g., "docker", "aws")
- `host` - Primary hostname

**Use cases:**
- Track WAL growth rate
- Detect write-heavy workloads
- Calculate throughput (bytes/sec)

### Replication Metrics (Primary View)

| Metric | Type | Description | Source |
|--------|------|-------------|--------|
| `pg_replication_lag_bytes` | Gauge | Lag from primary's current LSN to replica's replay LSN (bytes) | `pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn)` from `pg_stat_replication` |

**Labels:**
- `cluster`, `environment`, `provider` (same as above)
- `application_name` - Replica identifier (e.g., "replica-1")
- `client_addr` - Replica IP address
- `state` - Replication state ("streaming", "catchup", etc.)
- `sync_state` - Synchronization mode ("async", "sync", "potential")

**Use cases:**
- Monitor lag from primary's perspective
- Detect slow replicas
- Alert on excessive lag

**Note:** Uses `pg_current_wal_lsn()` instead of `sent_lsn` to capture total lag including sender delays on primary.

### Replication Metrics (Replica View)

| Metric | Type | Description | Source |
|--------|------|-------------|--------|
| `pg_replica_lag_bytes` | Gauge | Lag from subscription's perspective (bytes) | `pg_wal_lsn_diff(latest_end_lsn, received_lsn)` from `pg_stat_subscription` |
| `pg_replica_lag_milliseconds` | Gauge | Time-based lag (milliseconds) | Calculated from `last_msg_send_time` and `last_msg_recv_time` |

**Labels:**
- `cluster`, `environment`, `provider` (same as above)
- `host` - Replica hostname
- `subscription_name` - Subscription identifier (e.g., "pgbench_sub_replica-1")

**Use cases:**
- Monitor lag from replica's perspective
- Detect network delays
- Track message delivery latency

---

## Configuration

### Environment Variables

#### Required (All Operations)

```bash
# PostgreSQL password
PG_PASSWORD=your-secure-password
```

#### Required (Primary Connection)

```bash
PG_PRIMARY_HOST=127.0.0.1      # Primary hostname or IP
PG_PRIMARY_PORT=5432           # Primary port (default: 5432)
PG_PRIMARY_DATABASE=pgbench    # Database name (default: postgres)
PG_PRIMARY_USER=postgres       # Username (default: postgres)
```

#### Optional (Replica Connections)

```bash
# Comma-separated lists (one entry per replica)
PG_REPLICA_HOSTS=127.0.0.1,127.0.0.1,127.0.0.1
PG_REPLICA_PORTS=5540,5541,5542
PG_REPLICA_DATABASE=pgbench    # Database name (default: postgres)
PG_REPLICA_USER=postgres       # Username (default: postgres)
```

**Note:** If no replicas configured, only primary metrics are collected.

#### Required (Grafana Cloud Push)

```bash
# Grafana Cloud Prometheus Remote Write endpoint
GRAFANA_CLOUD_ENDPOINT=https://prometheus-prod-XX-prod-us-west-0.grafana.net/api/prom/push

# Grafana Cloud instance ID (username)
GRAFANA_CLOUD_USER=1234567

# Grafana Cloud API key (password)
GRAFANA_CLOUD_API_KEY=glc_eyJrIjoiYWJjMTIz...
```

#### Optional (Metric Labels)

```bash
TELEMETRY_CLUSTER=pg-telemetry-lab    # Cluster identifier
TELEMETRY_ENVIRONMENT=dev             # Environment (dev, staging, prod)
TELEMETRY_PROVIDER=docker             # Provider (docker, aws, gcp, onprem)
```

**Defaults:** If not set, uses "pg-telemetry-lab", "dev", and "unknown".

---

## Usage

### Local JSON Collection

**Single-shot to stdout:**
```bash
./telemetryctl telemetry collect
```

Output:
```json
{
  "timestamp": "2024-01-15T10:30:45Z",
  "primary": {
    "host": "127.0.0.1",
    "current_lsn_bytes": 123456789,
    "connections": [...]
  },
  "replicas": [...]
}
```

**Continuous collection to file (JSONL):**
```bash
./telemetryctl telemetry collect \
  --interval 5s \
  --duration 5m \
  --pretty=false \
  --output metrics.jsonl
```

**Analyze with jq:**
```bash
# Extract lag for replica-1
cat metrics.jsonl | jq '.replicas[] | select(.host | contains("5540")) | .lag_milliseconds'

# Calculate average lag
cat metrics.jsonl | jq -s 'map(.replicas[0].lag_milliseconds) | add / length'

# Find max lag
cat metrics.jsonl | jq -s 'map(.replicas[0].lag_milliseconds) | max'
```

### Grafana Cloud Push

**Single metric push:**
```bash
./telemetryctl telemetry collect --writer grafana
```

**Continuous push (recommended):**
```bash
./telemetryctl telemetry collect \
  --writer grafana \
  --interval 5s \
  --duration 10m
```

**During benchmark:**
```bash
./telemetryctl benchmark local \
  --duration 60 \
  --clients 50 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 5s
```

---

## Grafana Cloud Setup

### 1. Create Account

1. Go to [grafana.com](https://grafana.com/)
2. Sign up for free account (includes 10K metrics series)
3. Create a stack (select region)

### 2. Get Credentials

1. Navigate to **Connections** → **Add new connection**
2. Search for **"Prometheus"** and select **"Hosted Prometheus metrics"**
3. Click **"Generate now"** to create API key
4. Copy the three values:
   - **Remote Write Endpoint** (URL ending in `/api/prom/push`)
   - **Username / Instance ID** (numeric ID)
   - **Password / API Key** (starts with `glc_`)

### 3. Configure Environment

Add to `.env`:
```bash
GRAFANA_CLOUD_ENDPOINT=https://prometheus-prod-67-prod-us-west-0.grafana.net/api/prom/push
GRAFANA_CLOUD_USER=2933003
GRAFANA_CLOUD_API_KEY=glc_eyJrIjoiYWJjMTIz...
```

### 4. Test Connection

```bash
./telemetryctl telemetry collect --writer grafana
```

Expected output:
```
📊 Collecting metrics from 1 primary, 2 replicas...
✅ Successfully pushed metrics to Grafana Cloud
```

---

## Grafana Dashboards

### Create Dashboard

1. Log in to Grafana Cloud
2. Navigate to **Dashboards** → **New** → **New Dashboard**
3. Click **Add visualization**
4. Select your **Prometheus data source**
5. Enter PromQL query

### Essential Queries

#### Replication Lag (Milliseconds)

```promql
# All replicas
pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"}

# Specific replica
pg_replica_lag_milliseconds{cluster="pg-telemetry-lab", host="127.0.0.1:5540"}

# By subscription name
pg_replica_lag_milliseconds{subscription_name=~"replica-.*"}
```

**Panel settings:**
- Visualization: Time series
- Unit: milliseconds (ms)
- Min: 0

#### Replication Lag (Bytes)

```promql
# From primary's view
pg_replication_lag_bytes{cluster="pg-telemetry-lab"}

# From replica's view
pg_replica_lag_bytes{cluster="pg-telemetry-lab"}

# Convert to MB
pg_replication_lag_bytes{cluster="pg-telemetry-lab"} / 1024 / 1024
```

**Panel settings:**
- Visualization: Time series
- Unit: bytes (IEC) or custom (MB)
- Min: 0

#### Primary LSN Growth Rate

```promql
# Bytes per second
rate(pg_primary_lsn_bytes{cluster="pg-telemetry-lab"}[1m])

# MB per minute
rate(pg_primary_lsn_bytes{cluster="pg-telemetry-lab"}[1m]) * 60 / 1024 / 1024

# Total increase in last minute
increase(pg_primary_lsn_bytes{cluster="pg-telemetry-lab"}[1m])
```

**Panel settings:**
- Visualization: Time series
- Unit: bytes/sec or custom (MB/min)

#### Slowest Replicas

```promql
# Top 3 slowest by lag
topk(3, pg_replication_lag_bytes{cluster="pg-telemetry-lab"})

# Bottom 3 fastest by lag
bottomk(3, pg_replication_lag_bytes{cluster="pg-telemetry-lab"})
```

**Panel settings:**
- Visualization: Time series or Bar chart
- Legend: {{application_name}}

#### Lag Aggregations

```promql
# Average lag across all replicas
avg(pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"})

# Max lag across all replicas
max(pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"})

# Min lag
min(pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"})

# Sum (total lag)
sum(pg_replica_lag_bytes{cluster="pg-telemetry-lab"})
```

#### Lag Rate of Change

```promql
# How fast lag is growing/shrinking
deriv(pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"}[1m])

# Positive = lag increasing, Negative = lag decreasing
```

### Example Dashboard Layout

**Row 1: Overview**
- Panel 1: Primary LSN (current position)
- Panel 2: Primary write rate (MB/min)
- Panel 3: Replica count (cardinality)

**Row 2: Replication Lag**
- Panel 1: Lag by replica (milliseconds) - time series
- Panel 2: Lag by replica (bytes) - time series
- Panel 3: Max/Avg/Min lag - stat panels

**Row 3: Details**
- Panel 1: Replication state (table)
- Panel 2: Lag rate of change (bar chart)

---

## Alerts

### Grafana Alerting

Create alerts for replication lag thresholds.

**High Lag Alert:**
```promql
pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"} > 1000
```

**Alert conditions:**
- Evaluation: Every 1m
- For: 2m (sustained)
- Severity: Warning
- Notification: Email/Slack

**Critical Lag Alert:**
```promql
pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"} > 5000
```

**Alert conditions:**
- Evaluation: Every 30s
- For: 1m
- Severity: Critical
- Notification: PagerDuty

**Replica Down Alert:**
```promql
absent(pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"})
```

---

## Provider-Specific Examples

### Docker (Local)

```bash
# Default for local development
PG_PRIMARY_HOST=127.0.0.1
PG_PRIMARY_PORT=5432
PG_REPLICA_HOSTS=127.0.0.1,127.0.0.1
PG_REPLICA_PORTS=5540,5541
TELEMETRY_PROVIDER=docker
```

### AWS RDS

```bash
# Production RDS cluster
PG_PRIMARY_HOST=mydb-cluster.cluster-abc123.us-west-2.rds.amazonaws.com
PG_PRIMARY_PORT=5432
PG_REPLICA_HOSTS=mydb-cluster.cluster-ro-abc123.us-west-2.rds.amazonaws.com
PG_REPLICA_PORTS=5432
TELEMETRY_PROVIDER=aws
TELEMETRY_ENVIRONMENT=production
```

### GCP Cloud SQL

```bash
# Cloud SQL with read replicas
PG_PRIMARY_HOST=35.123.45.67  # Primary public IP
PG_PRIMARY_PORT=5432
PG_REPLICA_HOSTS=35.123.45.68,35.123.45.69  # Replica IPs
PG_REPLICA_PORTS=5432,5432
TELEMETRY_PROVIDER=gcp
TELEMETRY_ENVIRONMENT=production
```

### On-Premise

```bash
# On-prem PostgreSQL cluster
PG_PRIMARY_HOST=db-primary.company.com
PG_PRIMARY_PORT=5432
PG_REPLICA_HOSTS=db-replica1.company.com,db-replica2.company.com
PG_REPLICA_PORTS=5432,5432
TELEMETRY_PROVIDER=onprem
TELEMETRY_ENVIRONMENT=production
```

---

## Troubleshooting

### No Metrics Collected

**Symptom:** Empty JSON output or no data in Grafana

**Solutions:**
```bash
# 1. Check replication is set up
docker exec pg-primary psql -U postgres -d pgbench -c \
  "SELECT COUNT(*) FROM pg_stat_replication"
# Should return > 0

# 2. Check environment variables
env | grep PG_

# 3. Test connection manually
psql -h $PG_PRIMARY_HOST -p $PG_PRIMARY_PORT -U $PG_PRIMARY_USER -d $PG_PRIMARY_DATABASE
```

### Grafana Cloud Push Fails

**Symptom:** `401 Unauthorized` or `failed to push`

**Solutions:**
```bash
# 1. Verify credentials
echo $GRAFANA_CLOUD_ENDPOINT
echo $GRAFANA_CLOUD_USER
echo $GRAFANA_CLOUD_API_KEY

# 2. Test with curl
curl -u "${GRAFANA_CLOUD_USER}:${GRAFANA_CLOUD_API_KEY}" \
  -H "Content-Type: application/x-protobuf" \
  "${GRAFANA_CLOUD_ENDPOINT}" \
  --data-binary ""

# 3. Regenerate API key if needed
# Go to Grafana Cloud → Connections → Generate new key
```

### Lag Always Shows 0

**Symptom:** `pg_replication_lag_bytes` is always 0

**Causes:**
- No write activity (run benchmark to generate writes)
- Replicas caught up (this is good!)
- Replication not configured (run `replication setup`)

**Test:**
```bash
# Generate writes
docker exec pg-primary psql -U postgres -d pgbench -c \
  "UPDATE pgbench_accounts SET abalance = abalance + 1"

# Check lag immediately
./telemetryctl telemetry collect
```

---

## Best Practices

1. **Collection Interval:** 5-10 seconds for production, 2 seconds for testing
2. **Retention:** Grafana Cloud free tier stores metrics for 14 days
3. **Cardinality:** Use consistent label values to avoid cardinality explosion
4. **Alerts:** Set thresholds based on your SLAs (e.g., 1s lag = warning, 5s = critical)
5. **Dashboards:** Create separate dashboards for overview, detailed analysis, and troubleshooting

---

## Next Steps

- [Setup Guide](SETUP.md) - Configure environment variables
- [Benchmarking Guide](BENCHMARKING.md) - Generate load to test metrics
- [Command Reference](USAGE.md) - Full telemetry command options
