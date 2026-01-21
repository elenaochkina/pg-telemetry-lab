package replication

import (
	"context"

	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg"
	dockerbenchmark "github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg/benchmark"

	"github.com/elenaochkina/pg-telemetry-lab/internal/replication"
)

// CreateSetupOptions creates Docker-specific replication setup options.
// This assembles all Docker-specific objects (provider, runner, topology) needed
// for the replication service layer to execute the replication workflow.
func CreateSetupOptions(
	cfg *config.Config,
	password string,
	ctx context.Context,
	initSchema bool,
	verify bool,
) replication.SetupOptions {
	// Create Docker provider
	provider := dockerpg.NewDockerPostgresProvider()

	// Create Docker benchmark runner
	runner := dockerbenchmark.NewDockerRunner(cfg.Postgres.Image, cfg.Postgres.Network)

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
