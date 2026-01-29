# Setup Guide

Complete installation and configuration guide for pg-telemetry-lab.

---

## Prerequisites

### Required
- **Go 1.21+** - [Download](https://golang.org/dl/)
- **Docker** - [Install Docker Desktop](https://www.docker.com/products/docker-desktop)
- **PostgreSQL password** - Set via environment variable

### Optional
- **Grafana Cloud account** - For metrics visualization ([Sign up free](https://grafana.com/auth/sign-up/create-user))
- **jq** - For JSON parsing in examples (`brew install jq` on macOS)

---

## Installation

### 1. Clone Repository

```bash
git clone https://github.com/elenaochkina/pg-telemetry-lab.git
cd pg-telemetry-lab
```

### 2. Build CLI

```bash
go build -o telemetryctl .
```

This creates the `telemetryctl` binary in the current directory.

**Verify build:**
```bash
./telemetryctl --help
```

### 3. Set PostgreSQL Password

**Option A: Export directly**
```bash
export PG_PASSWORD=your-secure-password
```

**Option B: Create .env file (recommended)**
```bash
cp .env.example .env
# Edit .env with your editor
```

The CLI automatically loads `.env` if present.

---

## Environment Configuration

### Required Variables

```bash
# PostgreSQL Password (required for all operations)
PG_PASSWORD=your-secure-password

# PostgreSQL Connection (required for telemetry collection)
PG_PRIMARY_HOST=127.0.0.1
PG_PRIMARY_PORT=5432
PG_PRIMARY_DATABASE=pgbench
PG_PRIMARY_USER=postgres

# Replica Connections (comma-separated, required if collecting replica metrics)
PG_REPLICA_HOSTS=127.0.0.1,127.0.0.1
PG_REPLICA_PORTS=5540,5541
PG_REPLICA_DATABASE=pgbench
PG_REPLICA_USER=postgres
```

### Optional Variables (Grafana Cloud)

```bash
# Grafana Cloud Prometheus Remote Write Endpoint
GRAFANA_CLOUD_ENDPOINT=https://prometheus-prod-XX-prod-us-west-0.grafana.net/api/prom/push
GRAFANA_CLOUD_USER=your-instance-id
GRAFANA_CLOUD_API_KEY=glc_your_api_key_here

# Telemetry Labels (optional, have defaults)
TELEMETRY_CLUSTER=pg-telemetry-lab
TELEMETRY_ENVIRONMENT=dev
TELEMETRY_PROVIDER=docker
```

**Finding your Grafana Cloud credentials:**
1. Log in to [Grafana Cloud](https://grafana.com/)
2. Go to **Connections** → **Add new connection** → **Hosted Prometheus metrics**
3. Click **Generate now** to create API key
4. Copy the endpoint URL, instance ID (user), and API key

---

## Verification Steps

### After Provisioning

**Check Docker containers:**
```bash
docker ps
```

Expected output:
```
CONTAINER ID   IMAGE         NAMES
abc123def456   postgres:16   pg-primary
def789ghi012   postgres:16   pg-replica-1
ghi345jkl678   postgres:16   pg-replica-2
```

**Check Docker network:**
```bash
docker network ls | grep pgnet
```

**Connect to primary:**
```bash
docker exec -it pg-primary psql -U postgres -c "SELECT version()"
```

**Check PostgreSQL settings:**
```bash
docker exec pg-primary psql -U postgres -c "SHOW wal_level"
# Should show: logical

docker exec pg-primary psql -U postgres -c "SHOW max_wal_senders"
# Should show: 32

docker exec pg-primary psql -U postgres -c "SHOW max_connections"
# Should show: 300 (if configured)
```

### After Replication Setup

**Check publication on primary:**
```bash
docker exec pg-primary psql -U postgres -d pgbench -c \
  "SELECT * FROM pg_publication"
```

Expected:
```
   pubname    | pubowner | puballtables | ...
--------------+----------+--------------+-----
 pgbench_pub  | 10       | f            | ...
```

**Check subscription on replica:**
```bash
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
  "SELECT subname, subenabled FROM pg_subscription"
```

Expected:
```
      subname       | subenabled
--------------------+------------
 pgbench_sub_replica-1 | t
```

**Check replication status (primary view):**
```bash
docker exec pg-primary psql -U postgres -d pgbench -c \
  "SELECT application_name, state, sync_state FROM pg_stat_replication"
```

Expected:
```
 application_name | state     | sync_state
------------------+-----------+------------
 replica-1        | streaming | async
 replica-2        | streaming | async
```

**Check replication status (replica view):**
```bash
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
  "SELECT subname, received_lsn, latest_end_lsn FROM pg_stat_subscription"
```

**Test data replication:**
```bash
# Insert on primary
docker exec pg-primary psql -U postgres -d pgbench -c \
  "INSERT INTO pgbench_accounts (aid, bid, abalance) VALUES (999, 1, 5000)"

# Verify on replica (wait 1 second)
sleep 1
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
  "SELECT * FROM pgbench_accounts WHERE aid = 999"
```

---

## Troubleshooting

### Build Errors

**Error: `package X is not in GOROOT`**
```bash
# Solution: Run go mod tidy
go mod tidy
go build -o telemetryctl .
```

**Error: `undefined: util.GetPassword`**
```bash
# Solution: Check all imports include internal/util
# This should be fixed in the codebase
```

### Docker Errors

**Error: `Cannot connect to Docker daemon`**
```bash
# Solution: Start Docker Desktop
# On macOS: Open Docker Desktop application
# On Linux: sudo systemctl start docker
```

**Error: `network pgnet not found`**
```bash
# Solution: Create network manually
docker network create pgnet
```

**Error: `port 5432 already in use`**
```bash
# Solution: Stop existing PostgreSQL
# Check what's using the port:
lsof -i :5432

# Option 1: Stop local PostgreSQL
brew services stop postgresql  # macOS
sudo systemctl stop postgresql # Linux

# Option 2: Change port in configs/local.docker.yaml
```

### Replication Errors

**Error: `could not connect to replica`**
```bash
# Solution: Check replica is running and ready
docker ps
docker logs pg-replica-1

# Wait for "database system is ready to accept connections"
# The retry mechanism should handle this automatically
```

**Error: `subscription already exists`**
```bash
# Solution: Drop subscription and retry
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
  "DROP SUBSCRIPTION IF EXISTS pgbench_sub_replica-1"

./telemetryctl replication setup
```

**Error: `publication already exists`**
```bash
# Solution: Drop publication and retry
docker exec pg-primary psql -U postgres -d pgbench -c \
  "DROP PUBLICATION IF EXISTS pgbench_pub"

./telemetryctl replication setup
```

### Benchmark Errors

**Error: `max_connections reached`**
```bash
# Solution: Increase max_connections in config
# Edit configs/local.docker.yaml:
postgres:
  max_connections: 300

# Destroy and re-provision
./telemetryctl destroy local
./telemetryctl provision local
```

**Error: `pgbench: could not connect to server`**
```bash
# Solution 1: Wait for database to be ready
# The retry mechanism should handle this

# Solution 2: Check primary is running
docker exec pg-primary psql -U postgres -c "SELECT 1"

# Solution 3: Increase resource limits
# Edit configs/local.docker.yaml:
resources:
  primary_cpu: 2.0  # Increase from 1.0
```

### Telemetry Errors

**Error: `PG_PRIMARY_HOST environment variable not set`**
```bash
# Solution: Set required environment variables
# Copy .env.example to .env and fill in values
cp .env.example .env
# Edit .env with your configuration
```

**Error: `failed to push to Grafana Cloud: 401`**
```bash
# Solution: Check Grafana Cloud credentials
# Verify these are correct in .env:
# - GRAFANA_CLOUD_ENDPOINT
# - GRAFANA_CLOUD_USER
# - GRAFANA_CLOUD_API_KEY

# Test with curl:
curl -u "${GRAFANA_CLOUD_USER}:${GRAFANA_CLOUD_API_KEY}" \
  "${GRAFANA_CLOUD_ENDPOINT}"
```

**Error: `no metrics collected`**
```bash
# Solution: Check replication is set up
docker exec pg-primary psql -U postgres -d pgbench -c \
  "SELECT COUNT(*) FROM pg_stat_replication"

# Should return > 0 if replicas are connected
```

---

## Provider-Specific Setup

### Docker (Local Development)

**Default configuration:** `configs/local.docker.yaml`

**Resource limits:**
```yaml
postgres:
  resources:
    primary_cpu: 2.0       # Primary CPU cores
    replica_cpu: 0.4       # Replica CPU cores (constrained for testing)
    benchmark_cpu: 3.0     # Benchmark container CPU
```

**Multiple replicas:**
```yaml
postgres:
  replicas:
    count: 16              # Up to 32 supported (WAL sender limit)
    base_port: 5540        # Ports: 5540, 5541, 5542, ...
```

### AWS RDS (Planned)

**Future configuration:** `configs/production.aws.yaml`

**Environment variables:**
```bash
# Use RDS endpoints instead of localhost
PG_PRIMARY_HOST=mydb.cluster-abc123.us-west-2.rds.amazonaws.com
PG_PRIMARY_PORT=5432
PG_REPLICA_HOSTS=mydb-replica1.cluster-abc123.us-west-2.rds.amazonaws.com,mydb-replica2.cluster-abc123.us-west-2.rds.amazonaws.com

TELEMETRY_PROVIDER=aws
```

**Note:** Provisioning and benchmarking won't work with AWS (requires Docker), but telemetry collection will work with any PostgreSQL.

---

## Next Steps

- [Command Reference](USAGE.md) - Learn all CLI commands
- [Replication Guide](REPLICATION.md) - Understand logical replication
- [Benchmarking Guide](BENCHMARKING.md) - Run performance tests
- [Telemetry Guide](TELEMETRY.md) - Set up metrics and dashboards
