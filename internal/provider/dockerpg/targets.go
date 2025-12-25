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
		// Host:     "localhost",
		Host: "127.0.0.1",
		Port:     cfg.Postgres.Primary.Port,
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
