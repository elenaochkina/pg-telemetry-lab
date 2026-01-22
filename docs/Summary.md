# Architecture Summary

This document explains the architecture of pg-telemetry-lab, focusing on provider abstraction, clean separation of concerns, and the factory pattern.

---

## Design Principles

The project separates infrastructure provisioning, connection topology, and database replication logic to keep the system **provider-agnostic** and easy to extend beyond local Docker to cloud providers (AWS).

---

## Architecture Layers

### 1. CLI Layer (Controllers)

**Location:** `cmd/`

The CLI layer handles user interaction and routing:

```
cmd/
├── cli.go                    # Router - dispatches commands
├── factory.go                # Factory - creates provider implementations
├── handlers.go               # Controllers - provision/destroy/benchmark
└── replicationHandlers.go    # Controllers - replication commands
```

**Responsibilities:**
- Parse CLI arguments and flags
- Load configuration files
- Call factory to create provider-specific implementations
- Delegate to service layer for business logic
- Handle errors and user output

**Pattern:**
```
User Input → Router (cli.go) → Controller (handlers.go)
                                    ↓
                               Factory (factory.go)
                                    ↓
                      Docker Provider | AWS Provider
                                    ↓
                               Service Layer
```

**Example - Replication Setup Controller:**
```go
func handleReplicationSetup(args []string) error {
    // 1. Parse CLI flags
    cfg := config.Load(configPath)

    // 2. Call factory (reads cfg.Provider to decide Docker/AWS)
    setupOpts, err := createReplicationSetup(cfg, password, ...)

    // 3. Delegate to service
    return replication.Run(setupOpts)
}
```

**Factory Pattern:**
```go
func createReplicationSetup(cfg *config.Config, ...) (SetupOptions, error) {
    switch cfg.Provider {
    case "docker":
        return dockerreplication.CreateSetupOptions(cfg, ...)
    case "aws":
        return awsreplication.CreateSetupOptions(cfg, ...)
    }
}
```

The factory reads the `provider` field from config and delegates to provider-specific packages. Controllers never know which provider they're using - they just call the factory, which returns a `SetupOptions` struct with all dependencies assembled.

---

### 2. Service Layer (Business Logic)

**Location:** `internal/replication/`

The service layer contains **provider-agnostic** business logic:

```
internal/replication/
├── setup.go         # Orchestration - replication workflow
├── publisher.go     # Publication management
├── subscriber.go    # Subscription management
└── wait.go          # Verification logic
```

**Responsibilities:**
- Orchestrate replication workflow (provision → init → publish → subscribe → verify)
- Execute SQL commands for publications/subscriptions
- Monitor replication progress via system catalogs
- Verify replication is working

**Key Feature:** Depends only on **interfaces**, not concrete implementations:
- `provider.PostgresProvider` (not `dockerpg.DockerProvider`)
- `benchmark.Runner` (not `benchmark.DockerRunner`)
- `topology.PGTarget` (provider-neutral connection info)

**Example - Replication Orchestration:**
```go
func Run(opts SetupOptions) error {
    // 1. Provision cluster (using provider interface)
    opts.Provider.ProvisionPostgres(opts.Config)

    // 2. Initialize schema (using benchmark runner interface)
    opts.BenchmarkRunner.Init(...)

    // 3. Create publication (pure SQL)
    pub := NewPublisher(primaryConn)
    pub.EnsurePublication(...)

    // 4. Create subscriptions (pure SQL)
    sub := NewSubscriber(replicaConn)
    sub.EnsureSubscription(...)

    // 5. Verify (pure SQL)
    verifyReplication(...)
}
```

**Why this works:** The service layer receives all dependencies from the controller via `SetupOptions`. It doesn't know if it's using Docker or AWS - it just uses the interfaces.

---

### 3. Provider Layer (Infrastructure)

**Location:** `internal/provider/`

The provider layer handles **infrastructure-specific** logic:

```
internal/provider/
├── provider.go           # PostgresProvider interface
└── dockerpg/
    ├── provider.go       # Docker implementation
    ├── state.go          # State management
    ├── targets.go        # Connection topology
    ├── conninfo.go       # Connection strings
    ├── benchmark/
    │   └── runner.go     # Docker-specific benchmark runner
    └── replication/
        └── setup.go      # Docker-specific replication setup
```

**Provider Interface:**
```go
type PostgresProvider interface {
    ProvisionPostgres(cfg *config.Config) error
    DestroyPostgres() error
}
```

**Docker Implementation:**
- Creates Docker containers
- Manages Docker networks
- Stores state in `.telemetry/local-state.json`
- Provides Docker-specific benchmark runner (runs pgbench in containers)
- Provides Docker-specific replication setup (assembles Docker topology)

**Future AWS Implementation:**
- Creates RDS instances
- Manages VPCs and security groups
- Uses AWS SDK
- Provides AWS-specific benchmark runner (runs pgbench on EC2)
- Provides AWS-specific replication setup (assembles RDS topology)

**Key Principle:** Each provider owns all its infrastructure code, including how to run benchmarks and set up replication for that specific provider.

---

### 4. Topology Layer (Connection Addressing)

**Location:** `internal/topology/`

Connection details are represented by a **provider-neutral** struct:

```go
type PGTarget struct {
    Label    string
    Host     string
    Port     int
    Database string
    User     string
}
```

Each provider supplies builders that translate its configuration into `PGTarget` values:

**Docker:**
```go
// Host-side connection (from CLI)
PrimaryTarget(cfg) → PGTarget{Host: "localhost", Port: 5432}

// Internal connection (from Docker container)
PrimaryTargetFromDocker(cfg) → PGTarget{Host: "pg-primary", Port: 5432}
```

**AWS (future):**
```go
// RDS endpoint
PrimaryTarget(cfg) → PGTarget{Host: "db.xyz.rds.amazonaws.com", Port: 5432}
```

**Why two types of targets?**
- **Host targets** - CLI/scripts connect from outside (localhost:5432)
- **Internal targets** - Containers connect from inside (pg-primary:5432)

This keeps all higher-level logic independent of Docker/AWS networking.

---

### 5. Connection Management

**Location:** `internal/db/connect.go`

A single helper creates database connections:

```go
db.Connect(ctx, PGTarget, password) → *pgx.Conn
```

**Responsibilities:**
- Build connection strings
- Retry connection attempts (30 second timeout)
- Verify connectivity with ping
- Handle connection timeouts

**Why separate?**
- Centralized retry logic
- One place to handle connection failures
- Works with any `PGTarget` (Docker, AWS, etc.)

---

### 6. Benchmark Layer

**Location:** `internal/benchmark/`

The benchmark layer provides a **provider-agnostic** interface for running pgbench benchmarks:

```
internal/benchmark/
└── runner.go        # Runner interface + PgBenchOptions (provider-agnostic)
```

**Runner Interface:**
```go
type Runner interface {
    Init(opts PgBenchOptions) error  // Initialize pgbench schema
    Run(opts PgBenchOptions) error   // Run benchmark workload
}
```

**PgBenchOptions:**
```go
type PgBenchOptions struct {
    HostName string  // Database host to connect to
    Port     int     // Database port
    User     string  // PostgreSQL user
    Database string  // Database name
    Duration int     // Benchmark duration (seconds)
    Clients  int     // Number of concurrent clients
    Scale    int     // Dataset size
    Progress int     // Progress interval
}
```

**Provider Implementations:**
- **Docker**: `internal/provider/dockerpg/benchmark/runner.go` - Runs pgbench in Docker containers
- **AWS** (future): `internal/provider/awspg/benchmark/runner.go` - Runs pgbench on EC2 instances

**Key Feature:** The interface accepts only `PgBenchOptions` (provider-agnostic), while each implementation handles provider-specific details internally (Docker image/network, AWS region/subnet, etc.).

---

### 7. Replication Logic (SQL Layer)

**Location:** `internal/replication/`

Pure SQL logic for logical replication:

- `EnsurePublication` (publisher) - Creates publication if not exists
- `EnsureSubscription` (subscriber) - Creates subscription if not exists
- `WaitUntilCaughtUp` - Monitors `pg_stat_subscription` for LSN sync

**Key Feature:** Depends only on a small DB interface:

```go
type DB interface {
    Exec(ctx, sql, args...) error
    Query(ctx, sql, args...) (Rows, error)
    QueryRow(ctx, sql, args...) Row
}
```

This interface is satisfied by `*pgx.Conn`, making the package:
- **Testable** (can mock DB interface)
- **Decoupled** (doesn't know about connections)
- **Provider-agnostic** (works with any PostgreSQL)

---

## Complete Flow Example

**User runs:** `telemetryctl replication setup`

```
1. Router (cli.go)
   ↓ Routes "replication" → handleReplication()

2. Controller (replicationHandlers.go)
   ↓ handleReplicationSetup()
   ↓ Parses flags, loads config
   ↓ cfg.Provider = "docker"

3. Factory (factory.go)
   ↓ createReplicationSetup(cfg)
   ↓ switch cfg.Provider:
   ↓   case "docker": dockerreplication.CreateSetupOptions()
   ↓   case "aws":    awsreplication.CreateSetupOptions()
   ↓
   ↓ Docker-specific setup (dockerpg/replication/setup.go):
   ↓   - Creates DockerProvider
   ↓   - Creates DockerRunner (from dockerpg/benchmark)
   ↓   - Builds Docker topology (PrimaryTarget, ReplicaTargets)
   ↓
   ↓ Returns SetupOptions{
   ↓   Provider: DockerProvider,
   ↓   BenchmarkRunner: DockerRunner,
   ↓   PrimaryTarget: {Host: "localhost", Port: 5432},
   ↓   ...
   ↓ }

4. Service (replication/setup.go)
   ↓ replication.Run(setupOpts)
   ↓ Uses provider interface (doesn't know it's Docker)
   ↓
   ↓ 1. opts.Provider.ProvisionPostgres()
   ↓ 2. opts.BenchmarkRunner.Init()
   ↓ 3. NewPublisher().EnsurePublication()
   ↓ 4. NewSubscriber().EnsureSubscription()
   ↓ 5. WaitUntilCaughtUp()
   ↓ 6. verifyReplication()

5. Infrastructure (provider/dockerpg)
   ↓ DockerProvider.ProvisionPostgres()
   ↓ Creates Docker containers
   ↓ Stores state
```

---

## Why This Design?

**✅ Separation of Concerns**
- CLI layer handles user interaction
- Service layer handles business logic
- Provider layer handles infrastructure

**✅ Provider Agnostic**
- Service layer doesn't know about Docker/AWS
- Factory pattern isolates provider selection
- Easy to add new providers

**✅ Testable**
- Service layer depends on interfaces
- Can mock providers for testing
- SQL logic is isolated

**✅ Maintainable**
- Clear boundaries between layers
- Changes to Docker don't affect replication logic
- Adding AWS doesn't touch service layer

**✅ Idempotent**
- Replication commands can be run multiple times
- Uses system catalogs to check existing state
- Safe to re-run

---

## Pattern Summary

**Controller → Factory → Service → Infrastructure**

1. **Controllers** parse CLI input and call factory
2. **Factory** reads config and creates provider-specific objects
3. **Service** uses interfaces to execute business logic
4. **Infrastructure** provides concrete implementations

This pattern enables:
- Clean architecture
- Provider abstraction
- Easy testing
- Future extensibility

---

## Adding a New Provider (AWS Example)

To add AWS support:

**1. Implement Provider Interface**
```go
// internal/provider/awspg/provider.go
type AWSProvider struct { ... }
func (p *AWSProvider) ProvisionPostgres(cfg) { ... }
func (p *AWSProvider) DestroyPostgres() { ... }
```

**2. Create Topology Functions**
```go
// internal/provider/awspg/targets.go
func PrimaryTarget(cfg) topology.PGTarget { ... }
func ReplicaTargets(cfg) []topology.PGTarget { ... }
```

**3. Create Benchmark Runner**
```go
// internal/provider/awspg/benchmark/runner.go
type AWSRunner struct {
    Region       string
    SubnetID     string
    InstanceType string
}
func (r *AWSRunner) Init(opts benchmark.PgBenchOptions) error { ... }
func (r *AWSRunner) Run(opts benchmark.PgBenchOptions) error { ... }
```

**4. Create Replication Setup**
```go
// internal/provider/awspg/replication/setup.go
func CreateSetupOptions(cfg, password, ctx, initSchema, verify) replication.SetupOptions {
    // Assemble AWS-specific dependencies
    provider := awspg.NewAWSProvider()
    runner := awsbenchmark.NewAWSRunner(...)
    // Build RDS topology
    return replication.SetupOptions{ ... }
}
```

**5. Update Factory**
```go
// cmd/factory.go
case "aws":
    return awsreplication.CreateSetupOptions(cfg, ...)
```

**6. Add Config Fields**
```yaml
# configs/production.aws.yaml
provider: aws
postgres:
  region: "us-west-2"
  instance_class: "db.r5.xlarge"
```

**No changes needed:**
- ❌ Service layer (`internal/replication/`)
- ❌ Controllers (`cmd/handlers.go`)
- ❌ SQL logic
- ❌ Benchmark interface (`internal/benchmark/`)

---

In summary:

**Providers create databases.**
**Topology describes how to reach them.**
**Replication operates purely at the SQL level.**
**Controllers orchestrate using the factory pattern.**
