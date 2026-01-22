package dockerpg

import (
	"fmt"

	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/topology"
)

// PrimaryTarget returns how to connect to the primary from the host machine.
// (Docker-specific assumption: host connects via localhost + published port.)
func PrimaryTarget(cfg *config.Config) topology.PGTarget {
	return topology.PGTarget{
		Label:    "primary",
		Host:     "127.0.0.1",
		Port:     cfg.Postgres.Primary.Port,
		Database: cfg.Postgres.Primary.Database,
		User:     cfg.Postgres.Primary.User,
	}
}

// PrimaryTargetFromDocker returns how to connect to the primary from inside a Docker container.
// (Docker-specific assumption: containers connect via Docker DNS name + container port 5432.)
func PrimaryTargetFromDocker(cfg *config.Config) topology.PGTarget {
	return topology.PGTarget{
		Label:    "primary",
		Host:     cfg.Postgres.Primary.HostName, // e.g., "pg-primary" (Docker network hostname)
		Port:     5432,                           // Internal container port
		Database: cfg.Postgres.Primary.Database,
		User:     cfg.Postgres.Primary.User,
	}
}

// ReplicaTargets returns how to connect to each replica from the host machine.
// Docker-specific assumption: base_port + i and host is localhost.
func ReplicaTargets(cfg *config.Config) []topology.PGTarget {
	n := cfg.Postgres.Replicas.Count
	out := make([]topology.PGTarget, 0, n)

	for i := 0; i < n; i++ {
		out = append(out, topology.PGTarget{
			Label:    fmt.Sprintf("replica-%d", i+1),
			Host:     "localhost",
			Port:     cfg.Postgres.Replicas.BasePort + i,
			Database: cfg.Postgres.Primary.Database,
			User:     cfg.Postgres.Primary.User,
		})
	}
	return out
}

// ReplicaTargetsFromDocker returns how to connect to all replicas from inside a Docker container.
// (Docker-specific assumption: containers connect via Docker DNS names + container port 5432.)
func ReplicaTargetsFromDocker(cfg *config.Config) []topology.PGTarget {
	n := cfg.Postgres.Replicas.Count
	out := make([]topology.PGTarget, 0, n)

	for i := 0; i < n; i++ {
		out = append(out, topology.PGTarget{
			Label:    fmt.Sprintf("replica-%d", i+1),
			Host:     fmt.Sprintf("%s%d", cfg.Postgres.Replicas.NamePrefix, i+1), // e.g., "pg-replica-1"
			Port:     5432, // Internal container port
			Database: cfg.Postgres.Primary.Database,
			User:     cfg.Postgres.Primary.User,
		})
	}
	return out
}
