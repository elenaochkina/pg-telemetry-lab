package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/elenaochkina/pg-telemetry-lab/internal/benchmark"
	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg"
	dockerbenchmark "github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg/benchmark"
	dockerreplication "github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg/replication"
	"github.com/elenaochkina/pg-telemetry-lab/internal/replication"
	"github.com/elenaochkina/pg-telemetry-lab/internal/telemetry"
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

// createTelemetryCollector creates a provider-agnostic telemetry collector.
// Connection details are read from environment variables, not config files.
// This makes telemetry collection work with Docker, AWS, GCP, or any PostgreSQL deployment.
func createTelemetryCollector(password string) (telemetry.Collector, error) {
	primary, err := buildPrimaryTargetFromEnv()
	if err != nil {
		return nil, fmt.Errorf("build primary target from env: %w", err)
	}

	replicas, err := buildReplicaTargetsFromEnv()
	if err != nil {
		return nil, fmt.Errorf("build replica targets from env: %w", err)
	}

	return telemetry.NewPostgresCollector(primary, replicas, password), nil
}

// buildPrimaryTargetFromEnv builds a PGTarget for the primary database from environment variables.
func buildPrimaryTargetFromEnv() (topology.PGTarget, error) {
	host := os.Getenv("PG_PRIMARY_HOST")
	if host == "" {
		return topology.PGTarget{}, fmt.Errorf("PG_PRIMARY_HOST environment variable not set")
	}

	portStr := os.Getenv("PG_PRIMARY_PORT")
	if portStr == "" {
		portStr = "5432" // Default PostgreSQL port
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return topology.PGTarget{}, fmt.Errorf("invalid PG_PRIMARY_PORT: %w", err)
	}

	database := os.Getenv("PG_PRIMARY_DATABASE")
	if database == "" {
		database = "postgres" // Default database
	}

	user := os.Getenv("PG_PRIMARY_USER")
	if user == "" {
		user = "postgres" // Default user
	}

	return topology.PGTarget{
		Label:    "primary",
		Host:     host,
		Port:     port,
		Database: database,
		User:     user,
	}, nil
}

// buildReplicaTargetsFromEnv builds PGTargets for replica databases from environment variables.
// Supports comma-separated lists for multiple replicas.
func buildReplicaTargetsFromEnv() ([]topology.PGTarget, error) {
	hostsStr := os.Getenv("PG_REPLICA_HOSTS")
	if hostsStr == "" {
		// No replicas configured - this is okay
		return []topology.PGTarget{}, nil
	}

	hosts := strings.Split(hostsStr, ",")
	for i := range hosts {
		hosts[i] = strings.TrimSpace(hosts[i])
	}

	// Parse ports (comma-separated, or use default for all)
	portsStr := os.Getenv("PG_REPLICA_PORTS")
	var ports []int
	if portsStr == "" {
		// Use default port 5432 for all replicas
		for range hosts {
			ports = append(ports, 5432)
		}
	} else {
		portStrs := strings.Split(portsStr, ",")
		if len(portStrs) != len(hosts) {
			return nil, fmt.Errorf("PG_REPLICA_PORTS count (%d) must match PG_REPLICA_HOSTS count (%d)", len(portStrs), len(hosts))
		}
		for _, portStr := range portStrs {
			port, err := strconv.Atoi(strings.TrimSpace(portStr))
			if err != nil {
				return nil, fmt.Errorf("invalid port in PG_REPLICA_PORTS: %w", err)
			}
			ports = append(ports, port)
		}
	}

	// Get database and user (same for all replicas)
	database := os.Getenv("PG_REPLICA_DATABASE")
	if database == "" {
		database = "postgres" // Default database
	}

	user := os.Getenv("PG_REPLICA_USER")
	if user == "" {
		user = "postgres" // Default user
	}

	// Build targets
	targets := make([]topology.PGTarget, len(hosts))
	for i, host := range hosts {
		targets[i] = topology.PGTarget{
			Label:    fmt.Sprintf("replica-%d", i+1),
			Host:     host,
			Port:     ports[i],
			Database: database,
			User:     user,
		}
	}

	return targets, nil
}

// Future: AWS provider setup would be in internal/provider/awspg/replication/setup.go
