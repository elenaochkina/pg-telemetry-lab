# Architecture Guide

Clean architecture for PostgreSQL replication monitoring with provider abstraction and dependency inversion.

---

## Design Philosophy

**Problem:** How to build a PostgreSQL monitoring tool that works with Docker, AWS RDS, and GCP Cloud SQL without duplicating code?

**Solution:** Separate infrastructure provisioning from monitoring logic using clean architecture patterns.

---

## Architecture Layers

```
┌─────────────────────────────────────────────────────┐
│ CLI Layer (cmd/)                                    │
│  • Parses commands and flags                        │
│  • Calls factory for provider implementations       │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│ Factory Layer (cmd/factory.go)                      │
│  • Reads config.Provider ("docker" or "aws")        │
│  • Creates provider-specific implementations        │
│  • Injects dependencies                             │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│ Service Layer (internal/replication, /telemetry)    │
│  • Provider-agnostic business logic                 │
│  • Depends on interfaces, not concrete types        │
│  • Orchestrates workflows                           │
└───────────────────┬─────────────────────────────────┘
                    │
┌───────────────────▼─────────────────────────────────┐
│ Provider Layer (internal/provider/dockerpg)         │
│  • Docker: Container management                     │
│  • AWS (future): RDS management                     │
│  • Infrastructure-specific implementation           │
└─────────────────────────────────────────────────────┘
```

**Pattern:** Controller → Factory → Service → Infrastructure

---

## Key Design Patterns

### 1. Factory Pattern

**Purpose:** Isolate provider selection to one place.

Factory reads `config.Provider` and creates appropriate implementations. Controllers never know if they're using Docker or AWS.

**Benefits:**
- Add new providers without touching service layer
- Single source of truth for provider logic
- Easy to test (mock the factory)

### 2. Dependency Inversion

**Purpose:** High-level modules depend on abstractions, not implementations.

Service layer depends on interfaces:
- `provider.PostgresProvider` (not `dockerpg.DockerProvider`)
- `benchmark.Runner` (not `dockerbenchmark.DockerRunner`)
- `topology.PGTarget` (value object, not Docker-specific config)

**Benefits:**
- Service layer is reusable across providers
- Testable (mock interfaces)
- Business logic decoupled from infrastructure

### 3. Provider-Agnostic Telemetry

**Purpose:** Monitor any PostgreSQL, regardless of how it was provisioned.

Telemetry uses environment variables, not config files:
```bash
PG_PRIMARY_HOST=127.0.0.1           # Docker
PG_PRIMARY_HOST=db.rds.amazonaws.com # AWS
```

**Benefits:**
- Works with Docker, AWS RDS, GCP Cloud SQL, on-prem
- No code duplication per provider
- Follows 12-factor app principles

---

## Code Organization

```
cmd/
├── cli.go                    # Command routing
├── factory.go                # Provider factory + dependency injection
├── handlers.go               # CLI controllers
├── replicationHandlers.go
└── telemetryHandlers.go

internal/
├── benchmark/                # Interface for pgbench runners
├── config/                   # YAML configuration
├── db/                       # Connection helper
├── provider/
│   ├── provider.go           # PostgresProvider interface
│   └── dockerpg/             # Docker implementation
│       ├── provider.go
│       ├── benchmark/        # Docker-specific runner
│       └── replication/      # Docker-specific setup
├── replication/              # Provider-agnostic logic
│   ├── setup.go              # Orchestration
│   ├── publisher.go          # Publication management
│   └── subscriber.go         # Subscription management
├── telemetry/                # Provider-agnostic monitoring
│   ├── postgres_collector.go
│   └── writer/
│       ├── json_writer.go
│       └── prometheus_writer.go
├── topology/                 # Connection addressing
└── util/                     # Shared utilities
```

---

## Key Architectural Decisions

### Why Separate Provisioning from Monitoring?

**Provisioning** (provider-specific):
- Docker: `docker run` commands, container management
- AWS: RDS API calls, VPC configuration

**Monitoring** (provider-agnostic):
- Connects to any PostgreSQL
- Queries `pg_stat_replication`, `pg_stat_subscription`
- Works with Docker, AWS, GCP, on-prem

**Decision:** Keep them separate. Telemetry should work regardless of infrastructure.

### Why Environment Variables for Telemetry?

**Alternative considered:** Config file with provider-specific sections.

**Rejected because:**
- Couples telemetry to provisioning
- Requires separate implementation per provider
- Less flexible (can't easily point at existing databases)

**Chosen approach:** Environment variables (12-factor app).

**Benefits:**
- Single telemetry implementation
- Works with any PostgreSQL
- Simple deployment

### Why Two Target Types (Host vs Internal)?

**Problem:** Docker containers use different hostnames than host machine.

**Examples:**
- From macOS: `localhost:5432` (port mapping)
- From container: `pg-primary:5432` (Docker network)

**Solution:** Provider-specific target builders handle networking complexity.

Each provider knows how to translate its configuration into connection details for different contexts.

### Why Retry with Exponential Backoff?

**Problem:** Replicas with 0.2-0.4 CPU start slowly, causing connection failures.

**Solution:** Retry with increasing delays (1s, 2s, 4s, 8s, max 5 attempts).

**Benefits:**
- Handles resource-constrained scenarios gracefully
- No manual intervention needed
- Enables extreme testing scenarios

### Why Millisecond Precision for Lag?

**Problem:** Second-precision misses transient lag spikes.

**Decision:** Changed from `LagSeconds` to `LagMilliseconds`.

**Benefits:**
- Captures short-lived spikes during initialization
- Better visualization in Grafana
- More accurate for fast replication

### Why Measure from `pg_current_wal_lsn()` Instead of `sent_lsn`?

**Problem:** `sent_lsn` doesn't account for sender lag on primary.

**Decision:** Use `pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn)`.

**Benefits:**
- Captures total end-to-end lag
- Includes sender delays on primary
- More accurate representation of replication health

---

## Extension Points

### Adding a New Provider

To add AWS RDS support:

1. **Implement interface:** `internal/provider/awspg/provider.go`
2. **Create topology builders:** `internal/provider/awspg/targets.go`
3. **Create benchmark runner:** `internal/provider/awspg/benchmark/runner.go`
4. **Create setup helper:** `internal/provider/awspg/replication/setup.go`
5. **Update factory:** Add `case "aws"` to `cmd/factory.go`
6. **Add config template:** `configs/production.aws.yaml`

**No changes needed:**
- Service layer (`internal/replication/`, `internal/telemetry/`)
- CLI controllers (`cmd/*Handlers.go`)
- Interfaces (`internal/benchmark/`, `internal/topology/`)

---

## Design Principles Summary

1. **Separation of Concerns** - CLI, business logic, and infrastructure are isolated
2. **Dependency Inversion** - Depend on interfaces, not concrete implementations
3. **Factory Pattern** - Centralize provider selection
4. **Provider Agnosticism** - Monitoring works with any PostgreSQL
5. **Fail-Safe Patterns** - Retries, timeouts, graceful degradation
6. **12-Factor App** - Environment-based configuration for runtime concerns

---

## Technical Highlights

- **Go interfaces** for provider abstraction
- **Factory pattern** for dependency injection
- **Context-based cancellation** for graceful shutdown
- **Exponential backoff** for reliability
- **Prometheus Remote Write** for metrics export
- **Logical replication** with publication/subscription
- **LSN tracking** for precise lag measurement

---

For implementation details, see the source code. For usage examples, see other documentation files.
