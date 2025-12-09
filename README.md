# 📊 pg-telemetry-lab

**A production-like Golang project for provisioning PostgreSQL clusters, running benchmarking workloads, and collecting operational telemetry.**

This project demonstrates:

- Infrastructure provisioning patterns (local + cloud-ready architecture)
- Go CLI applications (`telemetryctl`)
- YAML-based configuration
- Provider abstraction (Docker now; cloud providers later)
- Docker-based Postgres clusters (primary + replicas)
- Foundation for logical replication, benchmarking, metrics collection, and Prometheus/Grafana integration

This repository is actively developed as part of a multi-week engineering roadmap.

---

## ✨ Features (Current)

### ✅ CLI Tool: `telemetryctl`
A Go-powered command-line utility that provisions PostgreSQL resources, runs benchmarks, and manages cluster lifecycle.

Supported commands:

- `provision local` — start primary + replicas on a custom Docker network  
- `destroy local` — remove provisioned containers  
- `benchmark local` — run pgbench inside a Docker container against the primary  

---

## 🚀 Getting Started

### Build the CLI

```bash
go build -o telemetryctl ./cmd/telemetryctl
```

# Provision Local PostgreSQL Cluster
```bash
./telemetryctl provision local --config local.config.example.yaml
```

# Verify network and containers:
```bash
docker network ls      # shows pgnet
docker ps              # shows pg-primary + replicas
``` 
# Run pgbench inside a temporary Docker container, not on the host:
```bash
./telemetryctl benchmark local \
  --config config.example.yaml \
  --duration 60 \
  --clients 20 \
  --scale 1 \
  --progress 5
  ```

# Benchmark Configuration Defaults
benchmark:
  scale: 1        # dataset size (scale 1 ≈ 100k rows)
  clients: 10     # number of concurrent clients (-c)
  duration: 60    # benchmark duration in seconds (-T)
  threads: 1      # pgbench threads (-j); not yet exposed in CLI
  progress: 5     # progress report interval in seconds (-P)

# Override defaults:
```bash
./telemetryctl benchmark local --scale 5 --clients 30 --duration 120
```


# Check that:
- pgbench init runs successfully (creates pgbench_* tables)
- progress lines show tps/latency
- password is masked in the logged docker command

# Destroy containers
```bash
./telemetryctl destroy local
```

# The project expects the following environment variable:
PG_PASSWORD=<your-password>

🛠 Roadmap:
- Real PostgreSQL replication (physical/logical)
- Metrics collector (WAL, LSN, replication stats, tuples)
- Prometheus exporter + Grafana dashboard
- Cloud provider support (AWS/GCP)