# Command Reference

Complete reference for all `telemetryctl` commands, flags, and usage examples.

---

## CLI Structure

```bash
telemetryctl <command> <subcommand> [flags]
```

**Available commands:**
- `provision` - Provision PostgreSQL clusters
- `destroy` - Destroy provisioned clusters
- `benchmark` - Run pgbench benchmarks
- `replication` - Manage logical replication
- `telemetry` - Collect and export metrics

---

## provision

Provision a PostgreSQL cluster (primary + replicas).

### Usage

```bash
telemetryctl provision <target> [flags]
```

### Arguments

- `<target>` - Deployment target (currently only `local` supported)

### Flags

- `--config <path>` - Path to config file (default: `configs/local.docker.yaml`)

### Examples

**Provision with default config:**
```bash
./telemetryctl provision local
```

**Provision with custom config:**
```bash
./telemetryctl provision local --config configs/custom.yaml
```

### What It Does

1. Creates Docker network (`pgnet`)
2. Starts primary container (port 5432)
3. Starts N replica containers (ports 5540+)
4. Saves state to `.telemetry/local.state.json`

### Output

```
Running: docker network create pgnet
Running: docker run -d --name pg-primary --network pgnet ...
Running: docker run -d --name pg-replica-1 --network pgnet ...
Running: docker run -d --name pg-replica-2 --network pgnet ...
✅ PostgreSQL cluster provisioned successfully
```

---

## destroy

Destroy a provisioned PostgreSQL cluster.

### Usage

```bash
telemetryctl destroy <target>
```

### Arguments

- `<target>` - Deployment target (currently only `local` supported)

### Examples

**Destroy local cluster:**
```bash
./telemetryctl destroy local
```

**Complete cleanup:**
```bash
./telemetryctl destroy local
docker network rm pgnet
```

### What It Does

1. Reads state from `.telemetry/local.state.json`
2. Removes primary container
3. Removes all replica containers
4. Deletes state file

**Note:** Does not remove the Docker network. Clean up with `docker network rm pgnet`.

---

## benchmark

Run pgbench benchmarks against the PostgreSQL cluster.

### Usage

```bash
telemetryctl benchmark <target> [flags]
```

### Arguments

- `<target>` - Deployment target (currently only `local` supported)

### Flags

**Benchmark parameters:**
- `--duration <seconds>` - Benchmark duration in seconds (default: `60`)
- `--clients <n>` - Number of concurrent clients (default: `10`)
- `--scale <n>` - pgbench scale factor, dataset size (default: `1`)
- `--progress <seconds>` - Progress reporting interval (default: `5`, `0` to disable)

**Telemetry parameters:**
- `--collect-telemetry` - Enable telemetry collection during benchmark (default: `false`)
- `--telemetry-interval <duration>` - Collection interval (default: `5s`)
- `--telemetry-writer <type>` - Writer type: `json` or `grafana` (default: `json`)
- `--telemetry-output <path>` - Output file for JSON writer (default: `benchmark-metrics.jsonl`)

**Configuration:**
- `--config <path>` - Path to config file (default: `configs/local.docker.yaml`)

### Examples

**Basic benchmark (60 seconds, 10 clients):**
```bash
./telemetryctl benchmark local
```

**Heavy load (2 minutes, 200 clients):**
```bash
./telemetryctl benchmark local --duration 120 --clients 200
```

**Large dataset (scale 50 = 5M rows):**
```bash
./telemetryctl benchmark local --scale 50 --clients 50 --duration 120
```

**Benchmark with JSON telemetry:**
```bash
./telemetryctl benchmark local \
  --duration 60 \
  --clients 20 \
  --collect-telemetry \
  --telemetry-output my-metrics.jsonl
```

**Benchmark with Grafana Cloud telemetry:**
```bash
./telemetryctl benchmark local \
  --duration 120 \
  --clients 50 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 5s
```

**No progress output:**
```bash
./telemetryctl benchmark local --progress 0
```

### What It Does

1. Creates benchmark container
2. Optionally starts telemetry collection in background
3. Initializes pgbench schema (`pgbench -i`)
4. Runs benchmark (`pgbench -c <clients> -T <duration>`)
5. Optionally stops telemetry collection
6. Removes benchmark container

### Output

```
🔧 Initializing pgbench schema...
dropping old tables...
creating tables...
generating data (client-side)...
done in 2.34 s

🚀 Running benchmark...
progress: 5.0 s, 234.5 tps, lat 42.654 ms stddev 12.345
progress: 10.0 s, 241.2 tps, lat 41.432 ms stddev 11.234
...
transaction type: <builtin: TPC-B (sort of)>
scaling factor: 1
query mode: simple
number of clients: 10
number of threads: 1
duration: 60 s
number of transactions actually processed: 14567
latency average = 41.234 ms
tps = 242.783333 (including connections establishing)
tps = 242.891234 (excluding connections establishing)

✅ Benchmark completed
```

---

## replication

Manage PostgreSQL logical replication.

### Usage

```bash
telemetryctl replication <subcommand> [flags]
```

### Subcommands

- `setup` - Setup logical replication (publication + subscriptions)

---

### replication setup

Setup logical replication: create publication on primary, create subscriptions on replicas, verify replication.

### Usage

```bash
telemetryctl replication setup [flags]
```

### Flags

- `--config <path>` - Path to config file (default: `configs/local.docker.yaml`)
- `--init-schema` - Initialize pgbench schema (default: `true`)
- `--no-init-schema` - Skip schema initialization
- `--verify` - Verify replication after setup (default: `true`)
- `--no-verify` - Skip verification
- `--timeout <duration>` - Overall operation timeout (default: `5m`)

### Examples

**Full setup with defaults:**
```bash
./telemetryctl replication setup
```

**Skip schema initialization (already exists):**
```bash
./telemetryctl replication setup --no-init-schema
```

**Skip verification (faster):**
```bash
./telemetryctl replication setup --no-verify
```

**Custom timeout (for large datasets):**
```bash
./telemetryctl replication setup --timeout 10m
```

**Custom config:**
```bash
./telemetryctl replication setup --config configs/custom.yaml
```

### What It Does

1. Provisions cluster if not already running
2. Initializes pgbench schema on primary (if `--init-schema`)
3. Creates publication on primary for specified tables
4. Initializes pgbench schema on replicas (if `--init-schema`)
5. Creates subscriptions on each replica
6. Waits for LSN synchronization
7. Verifies replication with test insert (if `--verify`)

### Output

```
🔧 Step 1/7: Provisioning cluster...
✅ Cluster provisioned

🔧 Step 2/7: Initializing pgbench schema on primary...
✅ Primary schema initialized

🔧 Step 3/7: Creating publication on primary...
✅ Publication created: pgbench_pub

🔧 Step 4/7: Initializing pgbench schema on replicas...
⏳ replica-1 not ready yet (attempt 1/5), retrying in 1s...
✅ Schema initialized on 2 replicas

🔧 Step 5/7: Creating subscriptions on replicas...
✅ Subscriptions created on 2 replicas

🔧 Step 6/7: Waiting for LSN sync...
⏳ Waiting for replicas to catch up (attempt 1/24)...
✅ All replicas synced

🔧 Step 7/7: Verifying replication...
✅ Data replicated to all replicas

✅ Replication setup complete
```

---

## telemetry

Collect and export PostgreSQL replication metrics.

### Usage

```bash
telemetryctl telemetry <subcommand> [flags]
```

### Subcommands

- `collect` - Collect replication lag metrics

---

### telemetry collect

Collect replication metrics from PostgreSQL and output to JSON or Grafana Cloud.

### Usage

```bash
telemetryctl telemetry collect [flags]
```

### Flags

**Output control:**
- `--writer <type>` - Writer type: `json` or `grafana` (default: `json`)
- `--output <path>` - Output file for JSON writer (default: stdout)
- `--pretty` - Pretty-print JSON output (default: `true`)

**Collection timing:**
- `--interval <duration>` - Collection interval for continuous polling (default: single shot)
- `--duration <duration>` - Total collection duration (requires `--interval`)

### Examples

**Single-shot pretty JSON to stdout:**
```bash
./telemetryctl telemetry collect
```

**Single-shot to file (JSONL):**
```bash
./telemetryctl telemetry collect --pretty=false --output metrics.jsonl
```

**Continuous collection every 5 seconds:**
```bash
./telemetryctl telemetry collect --interval 5s --output metrics.jsonl
```

**Collect for 2 minutes:**
```bash
./telemetryctl telemetry collect --interval 5s --duration 2m --output metrics.jsonl
```

**Push single metric to Grafana Cloud:**
```bash
./telemetryctl telemetry collect --writer grafana
```

**Continuous push to Grafana Cloud:**
```bash
./telemetryctl telemetry collect --writer grafana --interval 5s --duration 5m
```

### What It Does

1. Reads connection details from environment variables
2. Connects to primary and replicas
3. Queries `pg_stat_replication` and `pg_stat_subscription`
4. Calculates lag metrics (bytes and milliseconds)
5. Outputs to JSON or pushes to Grafana Cloud

### Output (JSON)

```json
{
  "timestamp": "2024-01-15T10:30:45Z",
  "primary": {
    "host": "127.0.0.1",
    "current_lsn_bytes": 123456789,
    "connections": [
      {
        "application_name": "replica-1",
        "client_addr": "172.18.0.3",
        "state": "streaming",
        "sent_lsn": "0/75A2F3C8",
        "write_lsn": "0/75A2F3C8",
        "flush_lsn": "0/75A2F3C8",
        "replay_lsn": "0/75A2F000",
        "lag_bytes": 968,
        "sync_state": "async"
      }
    ]
  },
  "replicas": [
    {
      "host": "127.0.0.1",
      "subscription_name": "pgbench_sub_replica-1",
      "received_lsn": "0/75A2F3C8",
      "latest_end_lsn": "0/75A2F3C8",
      "lag_bytes": 0,
      "lag_milliseconds": 5.2,
      "last_msg_send_time": "2024-01-15T10:30:44Z",
      "last_msg_recv_time": "2024-01-15T10:30:45Z"
    }
  ]
}
```

### Environment Variables Required

```bash
# Required for all operations
PG_PASSWORD=your-password

# Primary connection
PG_PRIMARY_HOST=127.0.0.1
PG_PRIMARY_PORT=5432
PG_PRIMARY_DATABASE=pgbench
PG_PRIMARY_USER=postgres

# Replica connections (comma-separated)
PG_REPLICA_HOSTS=127.0.0.1,127.0.0.1
PG_REPLICA_PORTS=5540,5541
PG_REPLICA_DATABASE=pgbench
PG_REPLICA_USER=postgres

# Grafana Cloud (for --writer grafana)
GRAFANA_CLOUD_ENDPOINT=https://prometheus-prod-XX.grafana.net/api/prom/push
GRAFANA_CLOUD_USER=your-instance-id
GRAFANA_CLOUD_API_KEY=glc_...

# Optional labels
TELEMETRY_CLUSTER=pg-telemetry-lab
TELEMETRY_ENVIRONMENT=dev
TELEMETRY_PROVIDER=docker
```

---

## Complete Workflows

### Full Test Workflow

```bash
# 1. Setup
export PG_PASSWORD=test123
go build -o telemetryctl .

# 2. Provision and configure replication
./telemetryctl replication setup

# 3. Run benchmark
./telemetryctl benchmark local --duration 30 --clients 10

# 4. Check metrics
./telemetryctl telemetry collect --pretty

# 5. Cleanup
./telemetryctl destroy local
docker network rm pgnet
```

### Benchmark with Grafana Monitoring

```bash
# 1. Setup with Grafana credentials
cat > .env <<EOF
PG_PASSWORD=test123
PG_PRIMARY_HOST=127.0.0.1
PG_PRIMARY_PORT=5432
PG_PRIMARY_DATABASE=pgbench
PG_PRIMARY_USER=postgres
PG_REPLICA_HOSTS=127.0.0.1,127.0.0.1
PG_REPLICA_PORTS=5540,5541
GRAFANA_CLOUD_ENDPOINT=https://prometheus-prod-XX.grafana.net/api/prom/push
GRAFANA_CLOUD_USER=your-instance-id
GRAFANA_CLOUD_API_KEY=glc_...
EOF

# 2. Build and setup replication
go build -o telemetryctl .
./telemetryctl replication setup

# 3. Run benchmark with telemetry to Grafana
./telemetryctl benchmark local \
  --duration 60 \
  --clients 20 \
  --scale 10 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 5s

# 4. View in Grafana Cloud
# Dashboard → Explore → Query: pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"}

# 5. Cleanup
./telemetryctl destroy local
docker network rm pgnet
```

### Heavy Load Testing

```bash
# Setup large dataset and many replicas
./telemetryctl destroy local  # Clean slate

# Edit configs/local.docker.yaml:
# replicas.count: 16
# max_connections: 300

./telemetryctl replication setup

# Run heavy benchmark with telemetry
./telemetryctl benchmark local \
  --scale 50 \
  --clients 200 \
  --duration 120 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 2s
```

---

## Tips and Best Practices

### Performance

- **Scale factor:** `scale 1` = ~100K rows (~16MB), `scale 50` = ~5M rows (~800MB)
- **Clients:** Start with 10-20, increase to 50-200 for heavy load
- **Duration:** 60s for quick tests, 300s+ for sustained load
- **Telemetry interval:** 5s for normal, 2s for detailed, 10s for long runs

### Resource Constraints

```yaml
# configs/local.docker.yaml
postgres:
  resources:
    primary_cpu: 2.0      # Full power for primary
    replica_cpu: 0.4      # Constrained to create lag
    benchmark_cpu: 3.0    # Heavy load generator
```

Lower replica CPU (0.1-0.4) creates more visible replication lag.

### Grafana Dashboards

After collecting metrics, create Grafana dashboards with queries like:
```promql
# Lag over time
pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"}

# Primary write rate
rate(pg_primary_lsn_bytes[1m]) * 60 / 1024 / 1024

# Slowest replica
topk(1, pg_replication_lag_bytes)
```

See [Telemetry Guide](TELEMETRY.md) for more PromQL examples.
