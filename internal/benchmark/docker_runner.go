package benchmark

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/elenaochkina/pg-telemetry-lab/internal/util"
)

// DockerRunner implements the Runner interface using Docker.
type DockerRunner struct {
	Image   string
	Network string
}

// NewDockerRunner creates a Docker-based pgbench runner.
func NewDockerRunner(image, network string) *DockerRunner {
	return &DockerRunner{
		Image:   image,
		Network: network,
	}
}

// Init prepares the database for benchmarking by running `pgbench -i`.
func (r *DockerRunner) Init(opts PgBenchOptions) error {
	fmt.Println("🔧 Initializing pgbench schema...")

	// Flags specific to initialization.
	pgbenchArgs := []string{
		"pgbench",
		"-i",
		"-s", strconv.Itoa(opts.Scale),
	}
	// Shared connection args.
	pgbenchArgs = append(pgbenchArgs, buildConnArgs(opts)...)

	if err := r.runPgbench(pgbenchArgs); err != nil {
		return fmt.Errorf("initialization failed: %w", err)
	}

	fmt.Println("✅ Initialization complete.")
	return nil
}

// Run executes the actual benchmark workload.
func (r *DockerRunner) Run(opts PgBenchOptions) error {
	fmt.Println("🚀 Running pgbench benchmark...")

	// Flags specific to running the workload.
	pgbenchArgs := []string{
		"pgbench",
		"-T", strconv.Itoa(opts.Duration),
		"-c", strconv.Itoa(opts.Clients),
	}
	if opts.Progress > 0 {
		pgbenchArgs = append(pgbenchArgs, "-P", strconv.Itoa(opts.Progress))
	}

	// Shared connection args.
	pgbenchArgs = append(pgbenchArgs, buildConnArgs(opts)...)

	if err := r.runPgbench(pgbenchArgs); err != nil {
		return fmt.Errorf("benchmark run failed: %w", err)
	}

	fmt.Println("✅ Benchmark run complete.")
	return nil
}

// runPgbench runs a pgbench command inside a Docker container on the configured network.
func (r *DockerRunner) runPgbench(pgbenchArgs []string) error {
	// Get password from environment (host-side).
	pw, err := util.GetRequiredEnv("PG_PASSWORD")
	if err != nil {
		return err
	}

	// Build the full docker command arguments.
	dockerArgs := []string{
		"run",
		"--rm",
		"--network", r.Network,
		"-e", "PGPASSWORD=" + pw, // inside container, pgbench reads PGPASSWORD
		r.Image,
	}
	dockerArgs = append(dockerArgs, pgbenchArgs...)

	// Mask sensitive env vars for printing.
	printArgs := util.MaskArgs(dockerArgs)

	fmt.Printf("Executing: docker %s\n", util.FormatArgs(printArgs))

	// Execute the command.
	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// buildConnArgs builds the common pgbench connection arguments.
func buildConnArgs(opts PgBenchOptions) []string {
	return []string{
		"-h", opts.Name,
		"-p", strconv.Itoa(opts.Port),
		"-U", opts.User,
		opts.Database,
	}
}

