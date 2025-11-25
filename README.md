# pg-telemetry-lab

Experiment project to learn PostgreSQL logical replication, database benchmarking,
and telemetry collection.

**Week 1 focus:**  
Build a Go CLI tool (`telemetryctl`) that provisions local PostgreSQL instances
(primary + replicas) using Docker and a YAML config file.

pg-telemetry-lab/
│
├── README.md
├── config.example.yaml
│
├── docs/
│   ├── week1-plan.md
│   └── architecture-overview.md   (optional placeholder)
│
├── cmd/
│   └── telemetryctl/
│       └── main.go                (empty placeholder for Day 2)
│
└── internal/
    ├── config/
    │   └── config.go              (empty placeholder for Day 2)
    │
    └── provider/
        ├── provider.go            (interface placeholder)
        │
        └── dockerpg/
            └── provider.go        (implementation placeholder)
