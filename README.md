# 📊 pg-telemetry-lab

**A production-ready Golang project for provisioning PostgreSQL clusters with logical replication, running benchmarks, and collecting operational telemetry.**

This project demonstrates:

- **Clean architecture** with provider abstraction (Docker now; AWS ready)
- **Factory pattern** for provider-specific implementations
- **Go CLI application** (`telemetryctl`) with unified command structure
- **YAML-based configuration** with provider-specific settings
- **PostgreSQL logical replication** setup and verification
- **Docker-based development** with production-ready patterns
- **pgbench benchmarking** for performance testing
- **Real-time telemetry** with Grafana Cloud integration (Prometheus Remote Write)

---

## ✨ Features

### ✅ Unified CLI: `telemetryctl`

A single command-line tool that manages the entire PostgreSQL cluster lifecycle.

**Commands:**
- `provision local` — Provision primary + replicas
- `destroy local` — Remove provisioned containers
- `benchmark local` — Run pgbench benchmarks
- `replication setup` — Setup logical replication (publication + subscriptions)
- `telemetry collect` — Collect replication lag metrics (JSON or Grafana Cloud)

### ✅ Logical Replication Support

Fully automated logical replication setup:
- Creates publication on primary for specified tables
- Creates subscriptions on all replicas
- Waits for replication catch-up
- Verifies data replication

### ✅ Telemetry Collection

Real-time replication lag monitoring with **provider-agnostic design**:
- Works with Docker, AWS RDS, GCP Cloud SQL, or any PostgreSQL
- Collects metrics from `pg_stat_replication` and `pg_stat_subscription`
- JSON/JSONL output for local analysis
- Prometheus Remote Write for Grafana Cloud
- Integrated with benchmark runs
- Tracks LSN position, lag in bytes and seconds
- Configuration via environment variables (no config file needed)

### ✅ Provider-Agnostic Architecture

Clean separation using factory pattern:
- **Controllers** (`cmd/handlers.go`) - Parse CLI input, call factory
- **Factory** (`cmd/factory.go`) - Create Docker/AWS implementations
- **Service** (`internal/replication/setup.go`) - Provider-agnostic business logic
- **Providers** (`internal/provider/dockerpg`) - Infrastructure code

---

## 🚀 Quick Start

### Prerequisites

- Go 1.21+
- Docker
- PostgreSQL password in environment

### 1. Clone and Build

```bash
git clone <repo-url>
cd pg-telemetry-lab

# Build CLI
go build -o telemetryctl .
```

### 2. Set Password

```bash
export PG_PASSWORD=your-secure-password
```

**Setup Environment Variables**

Create a `.env` file in the project root:

```bash
# PostgreSQL Password
PG_PASSWORD=your-secure-password

# PostgreSQL Connection (for telemetry - provider-agnostic)
PG_PRIMARY_HOST=127.0.0.1
PG_PRIMARY_PORT=5432
PG_PRIMARY_DATABASE=pgbench
PG_PRIMARY_USER=postgres
PG_REPLICA_HOSTS=127.0.0.1,127.0.0.1
PG_REPLICA_PORTS=5540,5541

# Grafana Cloud (optional, for telemetry push)
GRAFANA_CLOUD_ENDPOINT=https://prometheus-prod-XX.grafana.net/api/prom/push
GRAFANA_CLOUD_USER=your-instance-id
GRAFANA_CLOUD_API_KEY=glc_your_api_key

# Telemetry Labels (optional)
TELEMETRY_CLUSTER=pg-telemetry-lab
TELEMETRY_ENVIRONMENT=dev
TELEMETRY_PROVIDER=docker
```

The CLI automatically loads `.env` for environment variables.

### 3. Provision Cluster

```bash
./telemetryctl provision local
```

**Creates:**
- Docker network (`pgnet`)
- Primary container (port 5432)
- 2 replica containers (ports 5540, 5541)

**Verify:**
```bash
docker network ls      # shows pgnet
docker ps              # shows pg-primary, pg-replica-1, pg-replica-2
docker exec pg-primary psql -U postgres -c "SELECT version()"
```

### 4. Setup Logical Replication

```bash
./telemetryctl replication setup
```

**What it does:**
1. Provisions cluster (if not running)
2. Initializes pgbench schema on primary
3. Creates publication on primary
4. Initializes pgbench schema on replicas
5. Creates subscriptions on replicas
6. Waits for LSN sync
7. Verifies with test data

**Options:**
```bash
# Skip schema initialization
./telemetryctl replication setup --no-init-schema

# Skip verification
./telemetryctl replication setup --no-verify

# Custom timeout
./telemetryctl replication setup --timeout 10m
```

**Verify replication:**
```bash
# Insert on primary
docker exec pg-primary psql -U postgres -d pgbench -c \
  "INSERT INTO pgbench_accounts (aid, bid, abalance) VALUES (999, 1, 5000)"

# Check replica
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
  "SELECT * FROM pgbench_accounts WHERE aid = 999"

# Check replication status
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
  "SELECT subname, received_lsn, latest_end_lsn FROM pg_stat_subscription"
```

### 5. Run Benchmark

```bash
./telemetryctl benchmark local --duration 120 --clients 20
```

**Options:**
```bash
--duration   Benchmark duration in seconds (default: 60)
--clients    Number of concurrent clients (default: 10)
--scale      Dataset size (default: 1)
--progress   Progress interval in seconds (default: 5)
```

**With telemetry collection:**
```bash
# Collect to JSON file during benchmark
./telemetryctl benchmark local \
  --duration 60 \
  --clients 20 \
  --telemetry-enabled \
  --telemetry-output benchmark-metrics.jsonl

# Push to Grafana Cloud during benchmark
./telemetryctl benchmark local \
  --duration 60 \
  --clients 20 \
  --telemetry-enabled \
  --telemetry-writer grafana \
  --telemetry-interval 5s
```

### 6. Collect Telemetry Metrics

**Provider-Agnostic Design:** Telemetry collection works with any PostgreSQL (Docker, AWS RDS, GCP Cloud SQL, on-prem). Connection details come from environment variables, not config files.

**Single-shot collection:**
```bash
# Pretty JSON to stdout
./telemetryctl telemetry collect

# JSONL to file
./telemetryctl telemetry collect --pretty=false --output metrics.jsonl
```

**Continuous polling:**
```bash
# Collect every 5 seconds to file
./telemetryctl telemetry collect --interval 5s --output metrics.jsonl

# Collect for 2 minutes
./telemetryctl telemetry collect --interval 5s --duration 2m --output metrics.jsonl
```

**Push to Grafana Cloud:**
```bash
# Ensure Grafana Cloud credentials are in .env file (see Setup section)
# Or export them:
export GRAFANA_CLOUD_ENDPOINT="https://prometheus-prod-XX.grafana.net/api/prom/push"
export GRAFANA_CLOUD_USER="your-instance-id"
export GRAFANA_CLOUD_API_KEY="glc_..."

# Push single metric
./telemetryctl telemetry collect --writer grafana

# Continuous push every 5 seconds
./telemetryctl telemetry collect --writer grafana --interval 5s

# Push for 2 minutes
./telemetryctl telemetry collect --writer grafana --interval 5s --duration 2m
```

**Using with AWS RDS or other providers:**
```bash
# Simply update environment variables to point to your PostgreSQL instances
export PG_PRIMARY_HOST=my-rds-instance.us-west-2.rds.amazonaws.com
export PG_PRIMARY_PORT=5432
export PG_REPLICA_HOSTS=replica1.us-west-2.rds.amazonaws.com,replica2.us-west-2.rds.amazonaws.com
export TELEMETRY_PROVIDER=aws

# Same command works
./telemetryctl telemetry collect --writer grafana --interval 5s
```

**Metrics collected:**
- `pg_primary_lsn_bytes` - Current LSN position on primary
- `pg_replication_lag_bytes` - Replication lag in bytes (from primary view)
- `pg_replica_lag_bytes` - Replication lag in bytes (from replica view)
- `pg_replica_lag_seconds` - Replication lag in seconds

**View in Grafana Cloud:**
```promql
# Replication lag in seconds
pg_replica_lag_seconds{cluster="pg-telemetry-lab"}

# Replication lag in bytes
pg_replica_lag_bytes{cluster="pg-telemetry-lab"}

# Primary LSN growth rate
rate(pg_primary_lsn_bytes{cluster="pg-telemetry-lab"}[1m])
```

### 7. Cleanup

```bash
./telemetryctl destroy local
docker network rm pgnet
```

---

## 🧪 Complete Test Workflow

### Basic Workflow

```bash
# Setup
export PG_PASSWORD=test123
go build -o telemetryctl .

# Provision and setup replication
./telemetryctl replication setup

# Verify replication
docker exec pg-primary psql -U postgres -d pgbench -c \
  "INSERT INTO pgbench_accounts (aid, bid, abalance) VALUES (888, 1, 3000)"

docker exec pg-replica-1 psql -U postgres -d pgbench -c \
  "SELECT * FROM pgbench_accounts WHERE aid = 888"

# Run benchmark
./telemetryctl benchmark local --duration 30 --clients 5

# Check lag
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
  "SELECT subname, received_lsn, latest_end_lsn FROM pg_stat_subscription"

# Cleanup
./telemetryctl destroy local
docker network rm pgnet
```

### With Telemetry Monitoring

```bash
# Setup with Grafana Cloud credentials
export PG_PASSWORD=test123
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
GRAFANA_CLOUD_API_KEY=glc_your_api_key
TELEMETRY_CLUSTER=pg-telemetry-lab
TELEMETRY_ENVIRONMENT=dev
TELEMETRY_PROVIDER=docker
EOF

go build -o telemetryctl .

# Provision and setup replication
./telemetryctl replication setup

# Run benchmark with telemetry to Grafana Cloud
./telemetryctl benchmark local \
  --duration 60 \
  --clients 20 \
  --telemetry-enabled \
  --telemetry-writer grafana \
  --telemetry-interval 5s

# View in Grafana Cloud: Dashboard → Explore → Query:
# pg_replica_lag_seconds{cluster="pg-telemetry-lab"}

# Or collect to local file for analysis
./telemetryctl benchmark local \
  --duration 30 \
  --clients 10 \
  --telemetry-enabled \
  --telemetry-output benchmark-run.jsonl

# Analyze local metrics
cat benchmark-run.jsonl | jq '.replicas[0].lag_seconds'

# Cleanup
./telemetryctl destroy local
docker network rm pgnet
```

---

## 📝 Configuration

### Docker Config: `configs/local.docker.yaml`

```yaml
version: 1
provider: docker
environment: local

postgres:
  image: "postgres:16"
  network: "pgnet"
  primary:
    name: "pg-primary"
    port: 5432
    database: "pgbench"
    user: "postgres"
  replicas:
    count: 2
    base_port: 5540
    name_prefix: "pg-replica-"

replication:
  enabled: true
  publication_name: "pgbench_pub"
  subscription_prefix: "pgbench_sub_"
  tables:
    - "public.pgbench_accounts"
    - "public.pgbench_branches"
    - "public.pgbench_tellers"
    - "public.pgbench_history"
  copy_data: false
  create_slot: true
  verify:
    poll_interval: "500ms"
    timeout: "2m"
    strict_lsn_match: true
```

### AWS Config Template: `configs/production.aws.yaml`

```yaml
version: 1
provider: aws
environment: production

postgres:
  region: "us-west-2"
  instance_class: "db.r5.xlarge"
  engine_version: "16.1"
  # AWS-specific fields (not yet implemented)
```

---

## 🏗️ Architecture

```
cmd/
├── cli.go                    # Router
├── factory.go                # Provider factory (Docker/AWS)
├── handlers.go               # Controllers
├── replicationHandlers.go    # Replication controllers
└── telemetryHandlers.go      # Telemetry controllers

internal/
├── replication/              # Service layer
│   ├── setup.go              # Orchestration
│   ├── publisher.go          # Publication management
│   ├── subscriber.go         # Subscription management
│   └── wait.go               # Verification
├── telemetry/                # Telemetry (provider-agnostic)
│   ├── collector.go          # Collector interface
│   ├── postgres_collector.go # PostgreSQL collector (works with any PostgreSQL)
│   ├── metrics.go            # Metric structs
│   └── writer/               # Metric writers
│       ├── json_writer.go    # JSON/JSONL output
│       └── prometheus_writer.go  # Grafana Cloud (Prometheus Remote Write)
├── provider/dockerpg/        # Docker provider
│   ├── provider.go           # Provisioning implementation
│   ├── benchmark/            # Docker benchmark runner
│   └── replication/          # Docker replication setup
├── benchmark/                # Benchmark abstractions
├── config/                   # Configuration
└── topology/                 # Connection topology
```

**Pattern:** Controller → Factory → Service → Infrastructure

See [Architecture Summary](docs/Summary.md) for detailed design.

---

## 🛣️ Roadmap

### Completed ✅
- [x] Local Docker provisioning
- [x] pgbench benchmarking
- [x] Logical replication setup
- [x] Provider-agnostic architecture
- [x] Replication verification
- [x] Unified CLI with factory pattern
- [x] Telemetry collection (replication lag metrics)
- [x] JSON/JSONL output for metrics
- [x] Grafana Cloud integration (Prometheus Remote Write)
- [x] Benchmark integration with telemetry

### In Progress 🚧
- [ ] AWS RDS provider implementation
- [ ] Additional metrics (WAL, connections, query stats)
- [ ] Pre-built Grafana dashboards
- [ ] Alert rules for replication lag

---

## 📖 Documentation

- [Architecture Summary](docs/Summary.md) - Design overview and patterns
- [CLI Consolidation Plan](docs/CLI-CONSOLIDATION-PLAN-REVISED.md) - Implementation details

---

## 🤝 Contributing

This is a learning/demonstration project. Contributions and feedback welcome!

---

Built to demonstrate clean architecture, provider abstraction patterns, and PostgreSQL logical replication in Go.
