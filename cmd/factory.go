package cmd

import (
	"context"
	"fmt"

	"github.com/elenaochkina/pg-telemetry-lab/internal/benchmark"
	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg"
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
		return benchmark.NewDockerRunner(cfg.Postgres.Image, cfg.Postgres.Network), nil
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
		return createDockerReplicationSetup(cfg, password, ctx, initSchema, verify), nil
	case "aws":
		return replication.SetupOptions{}, fmt.Errorf("AWS provider not yet implemented")
	default:
		return replication.SetupOptions{}, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}

// createDockerReplicationSetup creates Docker-specific replication setup options.
// This is where all Docker-specific objects are created and assembled.
func createDockerReplicationSetup(
	cfg *config.Config,
	password string,
	ctx context.Context,
	initSchema bool,
	verify bool,
) replication.SetupOptions {
	// Create Docker provider
	provider := dockerpg.NewDockerPostgresProvider()

	// Create Docker benchmark runner
	runner := benchmark.NewDockerRunner(cfg.Postgres.Image, cfg.Postgres.Network)

	// Get Docker topology
	primaryHost := dockerpg.PrimaryTarget(cfg)
	replicasHost := dockerpg.ReplicaTargets(cfg)
	primaryInternal := dockerpg.PrimaryTargetFromDocker(cfg)
	replicasInternal := dockerpg.ReplicaTargetsFromDocker(cfg)
	publisherConnInfo := dockerpg.PublisherConnInfo(cfg, cfg.Postgres.Primary.User, password)

	return replication.SetupOptions{
		Config:                 cfg,
		Password:               password,
		Provider:               provider,
		BenchmarkRunner:        runner,
		PrimaryTargetHost:      primaryHost,
		ReplicaTargetsHost:     replicasHost,
		PrimaryTargetInternal:  primaryInternal,
		ReplicaTargetsInternal: replicasInternal,
		PublisherConnInfo:      publisherConnInfo,
		InitSchema:             initSchema,
		Verify:                 verify,
		Context:                ctx,
	}
}

// Future: createAWSReplicationSetup would go here
// func createAWSReplicationSetup(...) replication.SetupOptions { ... }
