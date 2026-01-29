# Benchmarking Guide

Comprehensive guide to performance testing PostgreSQL clusters with pgbench and monitoring replication lag under load.

---

## Overview

This project uses **pgbench** (PostgreSQL's built-in benchmarking tool) to generate write-heavy workloads and measure replication lag under various conditions.

**Goals:**
- Generate realistic transactional workload (TPC-B-like)
- Create visible replication lag by constraining resources
- Measure lag during initialization and steady-state phases
- Test scalability with multiple replicas

---

## pgbench Basics

### What is pgbench?

pgbench is PostgreSQL's official benchmarking tool that simulates TPC-B-like workload:
- Multiple concurrent clients running transactions
- Mix of SELECT, UPDATE, INSERT operations
- Simulates banking transactions

### Test Phases

1. **Initialization** (`pgbench -i`): Creates schema and generates test data
2. **Benchmark** (`pgbench -c <clients> -T <duration>`): Runs workload

### Schema

pgbench creates 4 tables:
- `pgbench_accounts` - Account records (majority of data, 100K rows per scale unit)
- `pgbench_branches` - Branch records (1 row per scale unit)
- `pgbench_tellers` - Teller records (10 rows per scale unit)
- `pgbench_history` - Transaction history (grows during benchmark)

**Scale factor:** Controls dataset size
- `scale 1` ≈ 100,000 rows ≈ 16 MB
- `scale 10` ≈ 1,000,000 rows ≈ 160 MB
- `scale 50` ≈ 5,000,000 rows ≈ 800 MB
- `scale 100` ≈ 10,000,000 rows ≈ 1.6 GB

---

## Configuration

### Benchmark Parameters

Edit `configs/local.docker.yaml`:

```yaml
postgres:
  image: "postgres:16"
  network: "pgnet"
  max_connections: 300  # Support high client counts

  resources:
    primary_cpu: 2.0      # Primary needs full resources
    replica_cpu: 0.4      # Constrain replicas to create lag
    benchmark_cpu: 3.0    # Benchmark container needs resources

  primary:
    name: "pg-primary"
    port: 5432
    database: "pgbench"
    user: "postgres"

  replicas:
    count: 16             # Test with many replicas
    base_port: 5540
    name_prefix: "pg-replica-"
```

### Key Settings

**max_connections:**
- Default: 100
- Recommendation: `<clients> + <replicas> + 50`
- Example: 200 clients + 16 replicas + 50 = 270, set to 300

**WAL settings:** (automatically configured)
- `wal_level: logical` - Required for logical replication
- `max_wal_senders: 32` - Supports up to 32 replicas
- `max_replication_slots: 32` - One slot per replica

**Resource limits:**
- `primary_cpu: 2.0` - Full power for primary
- `replica_cpu: 0.1-0.4` - Constrain to create lag
- `benchmark_cpu: 3.0` - Generate heavy load

---

## Testing Strategies

### Strategy 1: Baseline Test (No Lag Expected)

**Goal:** Verify replication works correctly with no constraints

**Configuration:**
```yaml
replicas:
  count: 2
resources:
  primary_cpu: 2.0
  replica_cpu: 2.0  # Full resources
```

**Command:**
```bash
./telemetryctl benchmark local \
  --scale 10 \
  --clients 10 \
  --duration 60 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 5s
```

**Expected result:** Lag < 10ms, stable

---

### Strategy 2: Resource-Constrained Replicas

**Goal:** Create visible lag by limiting replica CPU

**Configuration:**
```yaml
replicas:
  count: 4
resources:
  primary_cpu: 2.0
  replica_cpu: 0.4  # Constrained
```

**Command:**
```bash
./telemetryctl benchmark local \
  --scale 50 \
  --clients 50 \
  --duration 120 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 2s
```

**Expected result:** Lag spikes during initialization, settles during steady-state

---

### Strategy 3: Many Replicas

**Goal:** Test scalability and WAL sender pressure

**Configuration:**
```yaml
replicas:
  count: 16  # Maximum with 32 WAL senders
resources:
  primary_cpu: 2.0
  replica_cpu: 0.4
```

**Command:**
```bash
./telemetryctl benchmark local \
  --scale 50 \
  --clients 100 \
  --duration 180 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 5s
```

**Expected result:** Higher lag due to many concurrent replication streams

---

### Strategy 4: Heavy Client Load

**Goal:** Generate maximum write throughput

**Configuration:**
```yaml
replicas:
  count: 4
resources:
  primary_cpu: 2.0
  replica_cpu: 0.4
max_connections: 300
```

**Command:**
```bash
./telemetryctl benchmark local \
  --scale 50 \
  --clients 200 \
  --duration 120 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 2s
```

**Expected result:** High transaction rate, sustained lag on constrained replicas

---

### Strategy 5: Large Dataset Initialization

**Goal:** Observe lag during heavy write phase (initialization)

**Configuration:**
```yaml
replicas:
  count: 8
resources:
  replica_cpu: 0.2  # Very constrained
```

**Command:**
```bash
./telemetryctl replication setup  # This does the initialization

# Then monitor during benchmark
./telemetryctl benchmark local \
  --scale 100 \
  --clients 50 \
  --duration 60 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 2s
```

**Expected result:** Significant lag during initialization as all replicas copy large dataset

**Note:** Telemetry now starts BEFORE pgbench initialization, so you'll capture the initialization lag automatically.

---

### Strategy 6: Batch Updates

**Goal:** Create sudden lag spikes with large batch operations

**Setup:**
```bash
# After replication setup, run manual batch update
docker exec pg-primary psql -U postgres -d pgbench -c \
  "UPDATE pgbench_accounts SET abalance = abalance + 1"

# Monitor lag
./telemetryctl telemetry collect --interval 1s --duration 2m
```

**Expected result:** Large lag spike immediately after batch update, gradually decreases

---

## Test Scenarios

### Scenario 1: Continuous Load Testing

```bash
# 1. Setup cluster with 8 replicas
./telemetryctl destroy local
# Edit configs/local.docker.yaml: replicas.count = 8, replica_cpu = 0.4
./telemetryctl replication setup

# 2. Run 10-minute benchmark with telemetry
./telemetryctl benchmark local \
  --scale 50 \
  --clients 100 \
  --duration 600 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 5s

# 3. Analyze in Grafana:
# - Lag over time (should show initialization spike)
# - Primary LSN growth rate
# - Slowest replicas
```

---

### Scenario 2: Comparison Test (1 vs 4 vs 8 vs 16 Replicas)

**Goal:** Fill the performance comparison table in README

```bash
# Test 1: 1 replica, full resources
./telemetryctl destroy local
# Edit config: count=1, replica_cpu=2.0
./telemetryctl replication setup
./telemetryctl benchmark local --scale 50 --clients 200 --duration 60 \
  --collect-telemetry --telemetry-output test-1-replica.jsonl

# Test 2: 4 replicas, constrained
./telemetryctl destroy local
# Edit config: count=4, replica_cpu=0.4
./telemetryctl replication setup
./telemetryctl benchmark local --scale 50 --clients 200 --duration 60 \
  --collect-telemetry --telemetry-output test-4-replicas.jsonl

# Test 3: 8 replicas, constrained
# ... (same pattern)

# Test 4: 16 replicas, constrained
# ... (same pattern)

# Analyze results with jq:
for f in test-*.jsonl; do
  echo "File: $f"
  cat $f | jq -s 'map(.replicas[].lag_milliseconds) | {avg: (add/length), max: max, min: min}'
done
```

---

### Scenario 3: Stress Test with Paused Replica

**Goal:** Create maximum lag by pausing replication

```bash
# 1. Setup and run benchmark
./telemetryctl replication setup
./telemetryctl benchmark local --scale 50 --clients 50 --duration 300 &
BENCH_PID=$!

# 2. Start telemetry monitoring
./telemetryctl telemetry collect --writer grafana --interval 5s --duration 5m &
TELEMETRY_PID=$!

# 3. After 1 minute, pause replica
sleep 60
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
  "ALTER SUBSCRIPTION pgbench_sub_replica-1 DISABLE"

# 4. Wait 2 minutes (lag accumulates)
sleep 120

# 5. Re-enable and watch catch-up
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
  "ALTER SUBSCRIPTION pgbench_sub_replica-1 ENABLE"

# 6. Wait for completion
wait $BENCH_PID
wait $TELEMETRY_PID
```

---

## Analyzing Results

### During Benchmark

**Watch primary write rate:**
```bash
docker exec pg-primary psql -U postgres -d pgbench -c "
  SELECT pg_current_wal_lsn();
"
# Run multiple times to see LSN increasing
```

**Watch replication lag:**
```bash
docker exec pg-primary psql -U postgres -d pgbench -c "
  SELECT
    application_name,
    state,
    pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) / 1024 / 1024 AS lag_mb
  FROM pg_stat_replication;
"
```

**Watch subscription lag:**
```bash
docker exec pg-replica-1 psql -U postgres -d pgbench -c "
  SELECT
    subname,
    pg_wal_lsn_diff(latest_end_lsn, received_lsn) / 1024 AS lag_kb
  FROM pg_stat_subscription;
"
```

### After Benchmark

**Analyze JSON output:**
```bash
# Average lag across all measurements
cat metrics.jsonl | jq -s 'map(.replicas[0].lag_milliseconds) | add / length'

# Max lag observed
cat metrics.jsonl | jq -s 'map(.replicas[0].lag_milliseconds) | max'

# Extract initialization phase (first 30 seconds)
head -n 6 metrics.jsonl | jq '.replicas[0].lag_milliseconds'

# Extract steady-state (after 30 seconds)
tail -n +7 metrics.jsonl | jq -s 'map(.replicas[0].lag_milliseconds) | add / length'

# Primary LSN growth
cat metrics.jsonl | jq -s '
  {
    start: .[0].primary.current_lsn_bytes,
    end: .[-1].primary.current_lsn_bytes,
    growth_mb: ((.[-1].primary.current_lsn_bytes - .[0].primary.current_lsn_bytes) / 1024 / 1024)
  }
'
```

**Grafana queries:**
```promql
# Average lag during test
avg_over_time(pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"}[10m])

# Max lag during test
max_over_time(pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"}[10m])

# Primary write rate (MB/min)
rate(pg_primary_lsn_bytes{cluster="pg-telemetry-lab"}[1m]) * 60 / 1024 / 1024
```

---

## Benchmarking Best Practices

### Resource Allocation

1. **Primary:** Always give full resources (2.0 CPU minimum)
2. **Replicas:** Start with 0.4 CPU, reduce to 0.2 or 0.1 for extreme lag
3. **Benchmark:** Match or exceed primary (3.0 CPU recommended)

### Client Configuration

1. **Start small:** 10 clients to verify setup
2. **Scale gradually:** 10 → 20 → 50 → 100 → 200
3. **Watch connections:** Ensure `max_connections` isn't exceeded
4. **Monitor primary:** Check CPU usage, don't max out

### Scale Factor

1. **Development:** scale 1-10 (fast initialization)
2. **Testing:** scale 50 (realistic dataset, ~800MB)
3. **Stress testing:** scale 100+ (large dataset, long initialization)

### Duration

1. **Quick tests:** 30-60 seconds
2. **Standard tests:** 120-300 seconds
3. **Sustained load:** 600+ seconds (10+ minutes)

### Telemetry Collection

1. **Interval:** 2-5 seconds (2s for detailed, 5s for overview)
2. **Start early:** Telemetry now starts before initialization
3. **Use Grafana:** Real-time visualization is invaluable
4. **Save JSON:** Keep local copy for analysis

---

## Common Issues

### Benchmark Fails to Start

**Symptom:** `could not connect to database`

**Solutions:**
```bash
# 1. Wait for database to be ready
# (retry mechanism should handle this automatically)

# 2. Check primary is running
docker exec pg-primary psql -U postgres -c "SELECT 1"

# 3. Check max_connections
docker exec pg-primary psql -U postgres -c "SHOW max_connections"
# Should be > clients + replicas
```

### No Visible Lag

**Symptom:** Lag always near 0ms

**Solutions:**
```bash
# 1. Reduce replica CPU
# Edit configs/local.docker.yaml: replica_cpu: 0.2

# 2. Increase client count
./telemetryctl benchmark local --clients 200

# 3. Use larger dataset
./telemetryctl benchmark local --scale 100

# 4. Add more replicas
# Edit configs/local.docker.yaml: count: 16
```

### Benchmark Container Dies

**Symptom:** Benchmark stops mid-run

**Solutions:**
```bash
# 1. Check Docker logs
docker logs pgbench-runner

# 2. Increase benchmark CPU
# Edit configs/local.docker.yaml: benchmark_cpu: 3.0

# 3. Reduce client count
./telemetryctl benchmark local --clients 50
```

### Replica Can't Keep Up

**Symptom:** Lag keeps growing, never stabilizes

**Solutions:**
```bash
# 1. This might be expected behavior! (goal achieved)
# 2. If unwanted, increase replica_cpu to 0.5 or 1.0
# 3. Reduce client count to lower write rate
# 4. Check replica isn't stuck: docker logs pg-replica-1
```

---

## Reproducibility

To reproduce results, document:

1. **Configuration:**
   - `replicas.count`
   - `replica_cpu`
   - `primary_cpu`
   - `max_connections`

2. **Benchmark parameters:**
   - `--scale`
   - `--clients`
   - `--duration`

3. **System:** macOS/Linux, Docker version, available RAM

4. **Collected metrics:**
   - Average lag (init phase)
   - Max lag (init phase)
   - Average lag (steady-state)
   - Primary LSN growth rate

---

## Next Steps

- [Command Reference](USAGE.md) - Detailed flag options
- [Telemetry Guide](TELEMETRY.md) - Analyzing metrics in Grafana
- [Setup Guide](SETUP.md) - Troubleshooting configuration
