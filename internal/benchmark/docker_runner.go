package benchmark

import (
	"bytes"
	"fmt"
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
		"-i",
		"-s", strconv.Itoa(opts.Scale),
	}
	// Shared connection args.
	pgbenchArgs = append(pgbenchArgs, buildConnArgs(opts)...)

	output, err := r.runPgbench(pgbenchArgs)

	// For now, just print the raw pgbench output.
	// Later parse this to extract TPS, latency, etc.
	fmt.Print(output)

	if err != nil {
		return fmt.Errorf("pgbench initialization failed: %w", err)
	}	

	fmt.Println("✅ Initialization complete.")
	return nil
}

// Run executes the actual benchmark workload.
func (r *DockerRunner) Run(opts PgBenchOptions) error {
	fmt.Println("🚀 Running pgbench benchmark...")

	// Flags specific to running the workload.
	pgbenchArgs := []string{
		"-T", strconv.Itoa(opts.Duration),
		"-c", strconv.Itoa(opts.Clients),
	}
	if opts.Progress > 0 {
		pgbenchArgs = append(pgbenchArgs, "-P", strconv.Itoa(opts.Progress))
	}

	// Shared connection args.
	pgbenchArgs = append(pgbenchArgs, buildConnArgs(opts)...)

	output, err := r.runPgbench(pgbenchArgs)
	fmt.Print(output)

	if err != nil {
		return fmt.Errorf("pgbench run failed: %w", err)
	}	

	fmt.Println("✅ Benchmark run complete.")
	return nil
}

// runPgbench runs a pgbench command inside a Docker container on the configured network.
// It returns the combined pgbench output (stdout + stderr) and an error, if any.
func (r *DockerRunner) runPgbench(pgbenchArgs []string) (string, error) {
	// Get password from environment (host-side).
	pw, err := util.GetRequiredEnv("PG_PASSWORD")
	if err != nil {
		return "",  err
	}

	// Build the full docker command arguments.
	dockerArgs := []string{
		"run",
		"--rm",
		"--network", r.Network,
		"-e", "PGPASSWORD=" + pw, // inside container, pgbench reads PGPASSWORD
		"--entrypoint", "pgbench", // override default entrypoint
		r.Image,
	}
	dockerArgs = append(dockerArgs, pgbenchArgs...)

	// Mask sensitive env vars for printing.
	printArgs := util.MaskArgs(dockerArgs)

	fmt.Printf("Executing: docker %s\n", util.FormatArgs(printArgs))

 	// Capture pgbench output (both stdout and stderr).
	var out bytes.Buffer
	
	cmd := exec.Command("docker", dockerArgs...)
	cmd.Stdout = &out
	cmd.Stderr = &out // pgbench prints progress to stderr

	if err := cmd.Run(); err != nil {
		// Return whatever pgbench printed, plus a wrapped error.
		return out.String(), fmt.Errorf("running pgbench: %w", err)
	}

	return out.String(), err

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

