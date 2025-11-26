package dockerpg

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider"
	"github.com/joho/godotenv"
)

var _ provider.PostgresProvider = (*DockerPostgresProvider)(nil)

type DockerPostgresProvider struct{}

func NewDockerPostgresProvider() *DockerPostgresProvider {
	_ = godotenv.Load()
	return &DockerPostgresProvider{}
}

// ProvisionPostgres starts the primary and replica Postgres containers using Docker.
func (dp *DockerPostgresProvider) ProvisionPostgres(cfg *config.Config) error {
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
		PrimaryContainer:  cfg.Postgres.Primary.Name,
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
	name := cfg.Postgres.Primary.Name
	port := cfg.Postgres.Primary.Port
	image := cfg.Postgres.Image
	envUser := cfg.Postgres.Primary.User

	envDatabase := cfg.Postgres.Primary.Database

	// Load password from environment (.env)
	envPassword := os.Getenv("PG_PRIMARY_PASSWORD")
	if envPassword == "" {
		return fmt.Errorf("PG_PRIMARY_PASSWORD is not set (put it in .env)")
	}

	removeContainerIfExists(name)

	args := []string{
		"run",
		"-d",
		"--name", name,
		"-e", fmt.Sprintf("POSTGRES_USER=%s", envUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", envPassword),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", envDatabase),
		"-p", fmt.Sprintf("%d:5432", port),
		image,
	}

	return runCommand("docker", args...)

}
func (dp *DockerPostgresProvider) runReplica(cfg *config.Config, replicaIndex int) error {
	name := replicaName(cfg, replicaIndex)
	hostPort := cfg.Postgres.Replicas.BasePort + replicaIndex
	image := cfg.Postgres.Image
	envUser := cfg.Postgres.Primary.User
	envDatabase := cfg.Postgres.Primary.Database

	// Load password from environment (.env)
	envPassword := os.Getenv("PG_PRIMARY_PASSWORD")
	if envPassword == "" {
		return fmt.Errorf("PG_PRIMARY_PASSWORD is not set (put it in .env)")
	}

	removeContainerIfExists(name)

	args := []string{
		"run",
		"-d",
		"--name", name,
		"-e", fmt.Sprintf("POSTGRES_USER=%s", envUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", envPassword),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", envDatabase),
		"-p", fmt.Sprintf("%d:5432", hostPort),
		image,
	}

	return runCommand("docker", args...)
}

func replicaName(cfg *config.Config, replicaIndex int) string {
	// index is 0-based; names are 1-based (pg-replica-1, pg-replica-2)
	return fmt.Sprintf("%s%d", cfg.Postgres.Replicas.NamePrefix, replicaIndex+1)
}

func runCommand(name string, args ...string) error {
	fmt.Printf("Running: %s %s\n", name, strings.Join(args, " "))

	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
func removeContainerIfExists(name string) {
	cmd := exec.Command("docker", "rm", "-f", name)
	out, err := cmd.CombinedOutput()

	// If the error is "No such container", ignore silently
	if err != nil {
		output := string(out)
		if strings.Contains(output, "No such container") {
			return
		}

		// Otherwise, warn the user
		fmt.Fprintf(os.Stderr, "warning: could not remove container %q: %v\n", name, err)
	}
}
