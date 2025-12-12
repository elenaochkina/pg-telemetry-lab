# 📈 Benchmark Results (pgbench) — Hardware & Performance

This document provides hardware specifications and performance measurements
generated using the `telemetryctl benchmark` command.

The goal is to evaluate:

- Baseline PostgreSQL throughput
- Impact of different pgbench parameters (clients, scale, duration)
- Effect of adding replicas on primary-node TPS
- Future comparison with telemetry agent enabled

---

## 🖥 Hardware / Environment

| Component | Details |
|----------|---------|
| Machine | MacBook Air M1 (ARM64) |
| CPU | Apple M1 (8-core: 4 performance, 4 efficiency) |
| RAM | 16 GB |
| Disk | 512 GB SSD |
| OS | macOS Sonoma 14.x |
| Docker | Docker Desktop v4.x |
| Postgres Image | `postgres:16` |
| pgbench Version | Inside docker: 16.11 |

Docker virtualization notes:
- Docker Desktop uses Apple Hypervisor Framework on M1
- pgbench is executed inside a short-lived container (`docker run --rm`)

---

## ⚡ Baseline Benchmark (Primary Only)

### Command

```bash
./telemetryctl benchmark local \
  --config config.example.yaml \
  --duration 60 \
  --clients 20 \
  --scale 1 \
  --progress 5
```

### Result
```bash
progress: 5.0 s, 5332.0 tps, lat 3.703 ms stddev 3.728, 0 failed
progress: 10.0 s, 5382.8 tps, lat 3.712 ms stddev 3.613, 0 failed
progress: 15.0 s, 5319.6 tps, lat 3.754 ms stddev 3.683, 0 failed
...
Average TPS ≈ 5350
```

### Interpretation:
Scale 1 is very small (100k rows). The primary is CPU-bound and performs well on M1.

### Client Scaling Test

| Clients (`-c`) | TPS   | Avg Latency |
| -------------- | ----- | ----------- |
| 1              | ~1100 | ~0.9 ms     |
| 5              | ~3600 | ~1.6 ms     |
| 10             | ~4900 | ~2.5 ms     |
| 20             | ~5350 | ~3.7 ms     |
| 50             | ~4800 | ~11 ms      |
| 100            | ~3200 | ~32 ms      |

### Interpretation:
- TPS increases until around 20 clients.
- After that, contention grows and TPS drops.
- Latency rises sharply once saturation begins.

🗃 Scale Factor Test (Dataset Size)

| Scale (`-s`) | Dataset Size | TPS (20 clients) |
| ------------ | ------------ | ---------------- |
| 1            | 100k rows    | ~5350            |
| 5            | 500k rows    | ~3900            |
| 10           | 1M rows      | ~3500            |
| 50           | 5M rows      | ~2400            |

### Interpretation:
- Larger datasets reduce TPS due to more I/O and cache misses.

📊 Summary:
- pgbench demonstrates predictable scaling behavior.
- With this foundation, the telemetry agent will measure:
    * LSN sent/received/replayed
    * WAL file pressure
    * replication lag
    * TPS and latency over time
- These results can be graphed in Prometheus/Grafana later.

✔ Next Steps
- Add threads (-j) support to the CLI
- Add the telemetry collector
- Add Grafana dashboards showing replication lag vs TPS
- Compare performance with telemetry enabled vs disabled