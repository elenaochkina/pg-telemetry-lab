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

```bash
go run ./cmd/telemetryctl provision local --config=config.example.yaml
go run ./cmd/telemetryctl destroy   local

