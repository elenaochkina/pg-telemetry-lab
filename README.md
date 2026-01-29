# 📊 pg-telemetry-lab

**Production-ready Go CLI for PostgreSQL cluster provisioning, logical replication setup, and real-time telemetry monitoring.**

Demonstrates clean architecture with provider abstraction, factory pattern, and comprehensive PostgreSQL replication lag monitoring. Tested with up to 16 replicas, 200 concurrent clients, and 5M row datasets.

---

## 🎯 Performance Results

### Replication Lag Under Heavy Load

**Test Configuration:**
- Dataset: pgbench scale 50 (5M rows, ~800MB)
- Duration: 20 minutes (1200 seconds)
- PostgreSQL: 16 with logical replication
- Test environment: macOS, Docker Desktop, M1 MacBook Air

### Performance by Client Count and Replica Count

| Replicas (CPU)  | 10 Clients | 50 Clients | 100 Clients | 200 Clients |
|-----------------|------------|------------|-------------|-------------|
| 🟢 **1 (0.4)**  | 4260       | 3944       | 3574        | 3076        |
| 🔵 **4 (0.4)**  | 3608       | 3692       | 3331        | 3008        |
| 🟡 **8 (0.4)**  | 3266       | 3466       | 3179        | 2736        |
| 🔴 **16 (0.4)** | 2645       | 2955       | 2713        | 2466        |

**Key Observations:**
- **Peak throughput:** 4260 TPS at 10 clients (1 replica baseline)
- **Critical threshold:** Between 4-8 replicas, replication overhead emerges even under high client load
  - At 200 clients: 1→4 = 2.2% degradation, 4→8 = 9.0% degradation, 8→16 = 9.9% degradation
  - **Total degradation at 16 replicas: 19.8%** (3076 → 2466 TPS at 200 clients)
- **Recommended limit:** Up to 4 replicas for constrained environments (0.4 CPU per replica)
- **Initialization lag:** Grows exponentially (0.5s → 1.4s → 5.6s → 12.7s for 1/4/8/16 replicas)
- **Steady-state lag:** Higher at 16 replicas (430-1466ms vs 10-37ms for 1-8 replicas) — **replicas struggle to keep up**
- **Color coding:** 🟢 1 replica | 🔵 4 replicas | 🟡 8 replicas | 🔴 16 replicas (all 0.4 CPU per replica)

→ See [Performance Analysis](docs/ANALYSIS.md) for comprehensive conclusions

**Key Findings:**
- ✅ Successfully handles 16 replicas with logical replication
- ✅ Resource-constrained replicas (0.4 CPU) test worst-case scenarios
- ✅ Exponential backoff retry handles slow replica startup
- ✅ Millisecond-precision lag tracking captures transient spikes
- ✅ Captures both initialization and steady-state workload metrics

→ See [Benchmarking Guide](docs/BENCHMARKING.md) for detailed testing methodology

---

## ✨ Key Features

- **Provider-Agnostic Telemetry** — Works with Docker, AWS RDS, GCP Cloud SQL, or any PostgreSQL (environment-based configuration)
- **Logical Replication** — Automated publication/subscription setup with verification
- **Real-Time Metrics** — Prometheus Remote Write to Grafana Cloud with millisecond precision
- **Clean Architecture** — Factory pattern with controller → service → provider separation
- **Resource Testing** — Configurable CPU limits (0.1-4.0) to simulate constrained environments
- **Unified CLI** — Single `telemetryctl` binary for provision, benchmark, replication, telemetry
- **Production Patterns** — Exponential backoff retries, context cancellation, graceful shutdown

---

## 🚀 Quick Start

### Prerequisites
- Go 1.21+
- Docker
- (Optional) Grafana Cloud account for metrics push

### 1. Build

```bash
git clone https://github.com/elenaochkina/pg-telemetry-lab.git
cd pg-telemetry-lab
go build -o telemetryctl .
```

### 2. Configure Environment

```bash
cp .env.example .env
# Edit .env with your PostgreSQL password and Grafana credentials
```

**Required variables:**
```bash
PG_PASSWORD=your-secure-password
PG_PRIMARY_HOST=127.0.0.1
PG_PRIMARY_PORT=5432
PG_REPLICA_HOSTS=127.0.0.1,127.0.0.1  # Comma-separated
PG_REPLICA_PORTS=5540,5541
```

**Optional (for Grafana Cloud):**
```bash
GRAFANA_CLOUD_ENDPOINT=https://prometheus-prod-XX.grafana.net/api/prom/push
GRAFANA_CLOUD_USER=your-instance-id
GRAFANA_CLOUD_API_KEY=glc_...
```

### 3. Setup Cluster with Replication

```bash
./telemetryctl replication setup
```

This provisions primary + replicas, initializes pgbench schema, and configures logical replication.

### 4. Run Benchmark with Telemetry

```bash
# Local JSON output
./telemetryctl benchmark local \
  --clients 20 --duration 60 \
  --collect-telemetry \
  --telemetry-output metrics.jsonl

# Push to Grafana Cloud
./telemetryctl benchmark local \
  --clients 20 --duration 60 \
  --collect-telemetry \
  --telemetry-writer grafana \
  --telemetry-interval 5s
```

### 5. Cleanup

```bash
./telemetryctl destroy local
docker network rm pgnet
```

→ See [Setup Guide](docs/SETUP.md) for detailed installation and troubleshooting

---

## 🏗️ Architecture

**Pattern:** Controller → Factory → Service → Provider

```
┌─────────────────────────────────────────────────────┐
│ CLI Layer (cmd/)                                    │
│  • Parses flags, loads config                       │
│  • Calls factory to create provider implementations │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│ Factory (cmd/factory.go)                            │
│  • createProvider(cfg) → Docker/AWS                 │
│  • createBenchmarkRunner(cfg) → Provider-specific   │
│  • createTelemetryCollector() → Provider-agnostic   │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│ Service Layer (internal/replication, internal/telemetry) │
│  • Provider-agnostic business logic                 │
│  • Orchestrates workflows                           │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│ Provider Layer (internal/provider/dockerpg/)        │
│  • Docker-specific: container management            │
│  • AWS-specific: RDS management (planned)           │
└─────────────────────────────────────────────────────┘
```

**Key Design Decisions:**
- **Dependency Inversion:** Services depend on `topology.PGTarget` interface, not concrete providers
- **Factory Pattern:** CLI doesn't know about Docker/AWS specifics
- **Provider-Agnostic Telemetry:** Uses environment variables instead of config files (works with any PostgreSQL)

**Directory Structure:**
```
cmd/                          # Controllers and factory
internal/
  ├── replication/            # Service: orchestration logic
  ├── telemetry/              # Service: provider-agnostic collectors
  ├── provider/dockerpg/      # Infrastructure: Docker implementation
  ├── benchmark/              # Abstractions for pgbench
  ├── config/                 # YAML configuration
  └── topology/               # Connection interfaces
```

→ See [Architecture Guide](docs/ARCHITECTURE.md) for detailed design patterns and code flow

---

## 📊 Metrics Collected

| Metric | Description | Source |
|--------|-------------|--------|
| `pg_primary_lsn_bytes` | Current WAL LSN position on primary (bytes) | `pg_current_wal_lsn()` |
| `pg_replication_lag_bytes` | Lag between primary write and replica replay (bytes) | `pg_stat_replication` |
| `pg_replica_lag_bytes` | Lag from replica's perspective (bytes) | `pg_stat_subscription` |
| `pg_replica_lag_milliseconds` | Time-based lag (milliseconds) | Calculated from `last_msg_send_time` |

**View in Grafana Cloud:**
```promql
# Lag in milliseconds
pg_replica_lag_milliseconds{cluster="pg-telemetry-lab"}

# Lag by replica
pg_replication_lag_bytes{application_name=~"replica.*"}

# Primary write rate (MB/min)
rate(pg_primary_lsn_bytes[1m]) * 60 / 1024 / 1024
```

→ See [Telemetry Guide](docs/TELEMETRY.md) for complete metric reference and PromQL queries

---

## 📚 Documentation

| Guide | Description |
|-------|-------------|
| [Setup Guide](docs/SETUP.md) | Installation, prerequisites, environment configuration, troubleshooting |
| [Command Reference](docs/USAGE.md) | Complete CLI command reference with all flags and examples |
| [Telemetry Guide](docs/TELEMETRY.md) | Metrics, Grafana dashboards, PromQL queries, provider-agnostic design |
| [Benchmarking Guide](docs/BENCHMARKING.md) | Performance testing strategies, load generation, resource tuning |
| [Replication Guide](docs/REPLICATION.md) | Logical replication deep dive, verification, troubleshooting |
| [Architecture Guide](docs/ARCHITECTURE.md) | Design patterns, code structure, provider abstraction |
| [Performance Analysis](docs/ANALYSIS.md) | Comprehensive analysis of replica scaling tests and key findings |

---

## 🛠️ Tech Stack

**Core:**
- Go 1.21+ (CLI, clean architecture)
- PostgreSQL 16 (logical replication, `pg_stat_replication`, `pg_stat_subscription`)
- Docker (local development, resource constraints)

**Testing & Monitoring:**
- pgbench (TPC-B workload generation)
- Prometheus Remote Write (metrics push)
- Grafana Cloud (visualization)
- PromQL (metric queries)

**Patterns:**
- Factory pattern for provider abstraction
- Dependency inversion (interface-based design)
- Context-based cancellation
- Exponential backoff retry
- Environment-based configuration (12-factor app)

---

## 🛣️ Roadmap

**Completed ✅**
- [x] Docker provider with CPU resource limits
- [x] Logical replication setup and verification
- [x] Provider-agnostic telemetry collection
- [x] Grafana Cloud integration (Prometheus Remote Write)
- [x] Millisecond-precision lag tracking
- [x] Retry mechanism for low-resource replicas
- [x] Support for 32 WAL senders and 300 connections

**Planned 🚧**
- [ ] AWS RDS provider implementation
- [ ] Pre-built Grafana dashboards (JSON export)
- [ ] Alert rules for replication lag thresholds
- [ ] Streaming replication support (in addition to logical)
- [ ] Additional metrics (connections, query stats, WAL segments)

---

## 🤝 Contributing

This is a learning/demonstration project. Contributions and feedback are welcome!

---

## 📄 License

MIT

---

Built to demonstrate clean architecture, provider abstraction, and PostgreSQL replication telemetry in Go.
