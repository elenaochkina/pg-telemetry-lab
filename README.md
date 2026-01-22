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

---

## ✨ Features

### ✅ Unified CLI: `telemetryctl`

A single command-line tool that manages the entire PostgreSQL cluster lifecycle.

**Commands:**
- `provision local` — Provision primary + replicas
- `destroy local` — Remove provisioned containers
- `benchmark local` — Run pgbench benchmarks
- `replication setup` — Setup logical replication (publication + subscriptions)

### ✅ Logical Replication Support

Fully automated logical replication setup:
- Creates publication on primary for specified tables
- Creates subscriptions on all replicas
- Waits for replication catch-up
- Verifies data replication

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

### 6. Cleanup

```bash
./telemetryctl destroy local
docker network rm pgnet
```

---

## 🧪 Complete Test Workflow

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
└── replicationHandlers.go    # Replication controllers

internal/
├── replication/              # Service layer
│   ├── setup.go              # Orchestration
│   ├── publisher.go          # Publication management
│   ├── subscriber.go         # Subscription management
│   └── wait.go               # Verification
├── provider/dockerpg/        # Docker provider
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

### In Progress 🚧
- [ ] AWS RDS provider implementation
- [ ] Metrics collection (WAL, LSN, replication lag)
- [ ] Prometheus exporter
- [ ] Grafana dashboards

---

## 📖 Documentation

- [Architecture Summary](docs/Summary.md) - Design overview and patterns
- [CLI Consolidation Plan](docs/CLI-CONSOLIDATION-PLAN-REVISED.md) - Implementation details

---

## 🤝 Contributing

This is a learning/demonstration project. Contributions and feedback welcome!

---

Built to demonstrate clean architecture, provider abstraction patterns, and PostgreSQL logical replication in Go.
