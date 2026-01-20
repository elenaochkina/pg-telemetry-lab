package dockerpg

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider"
	"github.com/elenaochkina/pg-telemetry-lab/internal/util"
)

var _ provider.PostgresProvider = (*DockerPostgresProvider)(nil)

type DockerPostgresProvider struct{}

func NewDockerPostgresProvider() *DockerPostgresProvider {
	return &DockerPostgresProvider{}
}

// ProvisionPostgres starts the primary and replica Postgres containers using Docker.
func (dp *DockerPostgresProvider) ProvisionPostgres(cfg *config.Config) error {
	if err := ensureNetwork(cfg.Postgres.Network); err != nil {
		return fmt.Errorf("ensuring docker network %q: %w", cfg.Postgres.Network, err)
	}
	if err := dp.runPrimary(cfg); err != nil {
		return fmt.Errorf("running primary Postgres container: %w", err)
	}
	replicas := make([]string, 0, cfg.Postgres.Replicas.Count)
	for i := 0; i < cfg.Postgres.Replicas.Count; i++ {
		if err := dp.runReplica(cfg, i); err != nil {
			return fmt.Errorf("running replica %d Postgres container: %w", i+1, err)
		}
		replicas = append(replicas, replicaName(cfg, i))
	}
	state := LocalState{
		PrimaryContainer:  cfg.Postgres.Primary.HostName,
		ReplicaContainers: replicas,
		Image:             cfg.Postgres.Image,
	}
	if err := SaveLocalState(state); err != nil {
		return fmt.Errorf("saving local state: %w", err)
	}
	return nil
}

// DestroyPostgres stops and removes the primary and replica Postgres containers
func (dp *DockerPostgresProvider) DestroyPostgres() error {
	state, err := LoadLocalState()
	if err != nil {
		return fmt.Errorf("loading local state: %w", err)
	}
	var errs []string
	if err := runCommand("docker", "rm", "-f", state.PrimaryContainer); err != nil {
		errs = append(errs, fmt.Sprintf("removing primary container %q: %v", state.PrimaryContainer, err))
	}
	for _, replica := range state.ReplicaContainers {
		if err := runCommand("docker", "rm", "-f", replica); err != nil {
			errs = append(errs, fmt.Sprintf("removing replica container %q: %v", replica, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors occurred while destroying containers: %s", strings.Join(errs, "; "))
	}
	_ = os.Remove(LocalStatePath)

	return nil
}

func (dp *DockerPostgresProvider) runPrimary(cfg *config.Config) error {
	pw, err := util.GetRequiredEnv("PG_PASSWORD")
	if err != nil {
		return err
	}

	args := []string{
		"run", "-d",
		"--name", cfg.Postgres.Primary.HostName,
		"--network", cfg.Postgres.Network,
		"-e", "POSTGRES_USER=" + cfg.Postgres.Primary.User,
		"-e", "POSTGRES_PASSWORD=" + pw,
		"-e", "POSTGRES_DB=" + cfg.Postgres.Primary.Database,
		"-p", fmt.Sprintf("%d:5432", cfg.Postgres.Primary.Port),

		cfg.Postgres.Image,

		// Override the default CMD and pass Postgres settings.
		"postgres",
		"-c", "wal_level=logical",
		"-c", "max_wal_senders=10",
		"-c", "max_replication_slots=10",
	}

	// Mask password when printing command
	printArgs := util.MaskArgs(args)

	fmt.Printf("Running: docker %s\n", strings.Join(printArgs, " "))

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (dp *DockerPostgresProvider) runReplica(cfg *config.Config, index int) error {
	// NOTE: At the moment this starts an additional standalone Postgres
	// container with the same image and credentials as the primary. It is
	// intended to become a replication replica in a follow-up change.
	pw, err := util.GetRequiredEnv("PG_PASSWORD")
	if err != nil {
		return err
	}

	// Derive replica name and host port from config and index.
	name := replicaName(cfg, index)
	hostPort := cfg.Postgres.Replicas.BasePort + index

	args := []string{
		"run", "-d",
		"--name", name,
		"--network", cfg.Postgres.Network, 
		"-e", "POSTGRES_USER=" + cfg.Postgres.Primary.User,
		"-e", "POSTGRES_PASSWORD=" + pw, 
		"-e", "POSTGRES_DB=" + cfg.Postgres.Primary.Database,
		"-p", fmt.Sprintf("%d:5432", hostPort),
		cfg.Postgres.Image,
	}

// TODO: configure this container as a real replica of the primary:
	//   - enable wal_level and replication settings on the primary
	//   - use pg_basebackup / primary_conninfo to clone data from primary
	//   - create and use replication slots
	//   - switch from "standalone" to streaming/logical replica

	if err := runCommand("docker", args...); err != nil {
		return fmt.Errorf("running replica container %q: %w", name, err)
	}
	return nil
}

func replicaName(cfg *config.Config, replicaIndex int) string {
	// index is 0-based; names are 1-based (pg-replica-1, pg-replica-2)
	return fmt.Sprintf("%s%d", cfg.Postgres.Replicas.NamePrefix, replicaIndex+1)
}

func runCommand(name string, args ...string) error {
	printArgs := util.MaskArgs(args)
	fmt.Printf("Running: %s %s\n", name, strings.Join(printArgs, " "))

	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
func ensureNetwork(network string) error {
	if network == "" {
		return fmt.Errorf("postgres.network must be set in config")
	}

	// Check if network already exists.
	checkCmd := exec.Command("docker", "network", "inspect", network)
	if err := checkCmd.Run(); err == nil {
		// Network already exists.
		return nil
	}

	// Create the network.
	fmt.Printf("Running: docker network create %s\n", network)
	createCmd := exec.Command("docker", "network", "create", network)
	createCmd.Stdout = os.Stdout
	createCmd.Stderr = os.Stderr
	return createCmd.Run()
}


