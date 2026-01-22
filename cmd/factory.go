package cmd

import (
	"context"
	"fmt"

	"github.com/elenaochkina/pg-telemetry-lab/internal/benchmark"
	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg"
	dockerbenchmark "github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg/benchmark"
	dockerreplication "github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg/replication"
	"github.com/elenaochkina/pg-telemetry-lab/internal/replication"
	"github.com/elenaochkina/pg-telemetry-lab/internal/topology"
)

// Factory functions create provider-specific implementations based on config.
// This is where the provider abstraction happens.

// createProvider creates the appropriate PostgresProvider implementation
// based on the provider field in the config.
func createProvider(cfg *config.Config) (provider.PostgresProvider, error) {
	switch cfg.Provider {
	case "docker":
		return dockerpg.NewDockerPostgresProvider(), nil
	case "aws":
		return nil, fmt.Errorf("AWS provider not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported provider: %s (supported: docker, aws)", cfg.Provider)
	}
}

// createBenchmarkRunner creates the appropriate benchmark.Runner implementation
// based on the provider field in the config.
func createBenchmarkRunner(cfg *config.Config) (benchmark.Runner, error) {
	switch cfg.Provider {
	case "docker":
		return dockerbenchmark.NewDockerRunner(
			cfg.Postgres.Image,
			cfg.Postgres.Network,
			cfg.Postgres.Resources.BenchmarkCPU,
		), nil
	case "aws":
		return nil, fmt.Errorf("AWS benchmark runner not yet implemented")
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

// getPrimaryTargetInternal returns the primary target for internal connections
// (e.g., from benchmark containers to database)
func getPrimaryTargetInternal(cfg *config.Config) (topology.PGTarget, error) {
	switch cfg.Provider {
	case "docker":
		return dockerpg.PrimaryTargetFromDocker(cfg), nil
	case "aws":
		return topology.PGTarget{}, fmt.Errorf("AWS topology not yet implemented")
	default:
		return topology.PGTarget{}, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

// createReplicationSetup creates a complete SetupOptions for the replication service.
// This assembles all provider-specific dependencies needed for replication setup.
func createReplicationSetup(
	cfg *config.Config,
	password string,
	ctx context.Context,
	initSchema bool,
	verify bool,
) (replication.SetupOptions, error) {
	switch cfg.Provider {
	case "docker":
		return dockerreplication.CreateSetupOptions(cfg, password, ctx, initSchema, verify), nil
	case "aws":
		return replication.SetupOptions{}, fmt.Errorf("AWS provider not yet implemented")
	default:
		return replication.SetupOptions{}, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

// Future: AWS provider setup would be in internal/provider/awspg/replication/setup.go
