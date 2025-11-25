# Week 1 – Provisioning Interface + Local Docker PostgreSQL Provider

## Goals

By the end of Week 1, I want:

- A Go CLI (`telemetryctl`) that can:
  - `provision local` – start a primary + N replicas in Docker
  - `destroy local` – remove the containers
- A simple YAML config describing the desired topology
- A local Docker PostgreSQL provider implementation (shelling out to `docker` CLI)
- Basic docs + example config

## CLI Interface (planned)

```bash
telemetryctl provision local --config config.yaml
telemetryctl destroy   local --config config.yaml
