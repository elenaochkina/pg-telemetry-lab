# 📊 pg-telemetry-lab

**A production-like Golang project for provisioning PostgreSQL clusters, running benchmarking workloads, and collecting operational telemetry.**

This project demonstrates:

- Infrastructure provisioning patterns (local + cloud-ready architecture)
- Go CLI applications
- YAML-driven configuration
- Provider abstraction (Docker now, AWS/GCP later)
- Docker-based Postgres clusters (primary + replicas)
- Foundation for logical replication, pgbench automation, metrics collection, and Prometheus integration

This is actively developed as part of a multi-week roadmap.

---

## ✨ Features (Current)

### ✅ CLI Tool: `telemetryctl`
A Go command-line tool to manage PostgreSQL infrastructure.

Supported commands:

# Build CLI
go build -o telemetryctl ./cmd/telemetryctl

# Provision primary + replicas
./telemetryctl provision local --config config.example.yaml

# Verify network and containers:
docker network ls      # shows pgnet
docker ps              # shows pg-primary + replicas

# Run benchmark
./telemetryctl benchmark local \
  --config config.example.yaml \
  --duration 60 --clients 20 --scale 1 --progress 5

# Check that:
# - pgbench init runs successfully (creates pgbench_* tables)
# - progress lines show tps/latency
# - password is masked in the logged docker command

# Destroy containers
./telemetryctl destroy local
