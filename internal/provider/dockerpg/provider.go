package dockerpg

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
)

type DockerPostgresProvider struct{}

func NewDockerPostgresProvider() *DockerPostgresProvider {
	return &DockerPostgresProvider{}
}

// ProvisionLocalPostgres starts the primary and replica Postgres containers using Docker.
func (dp *DockerPostgresProvider) ProvisionLocalPostgres(cfg *config.Config) error {
	if err := dp.runPrimary(cfg); err != nil {
		return fmt.Errorf("running primary Postgres container: %w", err)
	}
	for i := 0; i < cfg.Postgres.Replicas.Count; i++ {
		if err := dp.runReplica(cfg, i); err != nil {
			return fmt.Errorf("running replica %d Postgres container: %w", i+1, err)
		}
	}
	return nil
}

// DestroyLocalPostgres stops and removes the primary and replica Postgres containers
func (dp *DockerPostgresProvider) DestroyLocalPostgres(cfg *config.Config) error {
	var (
		errs       []string
		primaryErr error
	)

	if err := runCommand("docker", "rm", "-f", cfg.Postgres.Primary.Name); err != nil {
		primaryErr = fmt.Errorf("removing primary Postgres container %q: %w",
			cfg.Postgres.Primary.Name, err)
		errs = append(errs, primaryErr.Error())
	}

	for i := 0; i < cfg.Postgres.Replicas.Count; i++ {
		name := replicaName(cfg, i)
		if err := runCommand("docker", "rm", "-f", name); err != nil {
			errs = append(errs,
				fmt.Sprintf("removing replica %d Postgres container %q: %v",
					i+1, name, err),
			)
		}
	}

	if len(errs) == 0 {
		return nil
	}

	// If primary failed, you might want to highlight that:
	if primaryErr != nil {
		return fmt.Errorf("destroy failed (primary and/or replicas): %s", strings.Join(errs, "; "))
	}

	return fmt.Errorf("destroy failed (replicas): %s", strings.Join(errs, "; "))
}


func (dp *DockerPostgresProvider) runPrimary(cfg *config.Config) error {
	name := cfg.Postgres.Primary.Name
	port := cfg.Postgres.Primary.Port
	image := cfg.Postgres.Image
	envUser := cfg.Postgres.Primary.User
	envPassword := cfg.Postgres.Primary.Password
	envDatabase := cfg.Postgres.Primary.Database

	removeContainerIfExists(name)

	args := []string{
		"run",
		"-d",
		"--name", name,
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", envPassword),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", envUser),
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
	envPassword := cfg.Postgres.Primary.Password
	envDatabase := cfg.Postgres.Primary.Database
	
	removeContainerIfExists(name)

	args := []string{
		"run",
		"-d",
		"--name", name,
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", envPassword),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", envUser),
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

