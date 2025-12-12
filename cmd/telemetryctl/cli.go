package telemetryctl

import (
	"flag"
	"fmt"

	"github.com/joho/godotenv"

	"github.com/elenaochkina/pg-telemetry-lab/internal/benchmark"
	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg"
)

// Run is the entry point for CLI logic. It takes os.Args[1:].
func Run(args []string) error {
	_ = godotenv.Load() // load .env for the entire CLI

	if len(args) < 2 {
		return fmt.Errorf("not enough arguments\n\n%s", usage())
	}

	cmd := args[0]    // provision | destroy | benchmark
	target := args[1] // local (later maybe cloud)

	fs := flag.NewFlagSet("telemetryctl", flag.ContinueOnError)

	var configPath string
	fs.StringVar(&configPath, "config", "config.yaml", "path to config file")

	// Benchmark-related flags.
	var duration int
	var clients int
	var scale int
	var progress int

	fs.IntVar(&duration, "duration", 60, "benchmark duration in seconds")
	fs.IntVar(&clients, "clients", 10, "number of concurrent clients")
	fs.IntVar(&scale, "scale", 1, "pgbench scale factor (dataset size)")
	fs.IntVar(&progress, "progress", 5, "pgbench progress interval in seconds (0 disables progress output)")

	if err := fs.Parse(args[2:]); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	//Load config only for commands that need it
	var cfg *config.Config
	var err error	
	if cmd == "provision" || cmd == "benchmark" {
		cfg, err = config.Load(configPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	switch cmd {
	case "provision":
		return handleProvision(target, cfg)

	case "destroy":
		return handleDestroy(target)

	case "benchmark":
		return handleBenchmark(target, cfg, duration, clients, scale, progress)

	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usage())
	}
}

func handleProvision(target string, cfg *config.Config) error {
	switch target {
	case "local":
		provider := dockerpg.NewDockerPostgresProvider()
		if err := provider.ProvisionPostgres(cfg); err != nil {
			return fmt.Errorf("provisioning local postgres: %w", err)
		}
		fmt.Println("✅ Local PostgreSQL cluster provisioned successfully.")
		return nil
	case "cloud":
		return fmt.Errorf("cloud target not implemented yet")
	default:
		return fmt.Errorf("unsupported target %q (only \"local\" is supported for now)", target)
	}
}

func handleDestroy(target string) error {
	switch target {
	case "local":
		provider := dockerpg.NewDockerPostgresProvider()
		if err := provider.DestroyPostgres(); err != nil {
			return fmt.Errorf("destroying local postgres: %w", err)
		}
		fmt.Println("✅ Local PostgreSQL cluster destroyed (containers removed).")
		return nil
	case "cloud":
		return fmt.Errorf("cloud target not implemented yet")
	default:
		return fmt.Errorf("unsupported target %q (only \"local\" is supported for now)", target)
	}
}

func handleBenchmark(target string, cfg *config.Config, duration, clients, scale, progress int) error {
	// Basic sane defaults/validation.
	if duration <= 0 {
		duration = 60
	}
	if clients <= 0 {
		clients = 10
	}
	if scale <= 0 {
		scale = 1
	}
	if progress < 0 {
		progress = 0
	}
	switch target {
	case "local":
		runner := benchmark.NewDockerRunner(
			cfg.Postgres.Image,
			cfg.Postgres.Network,
		)

		opts := benchmark.PgBenchOptions{
			HostName:     cfg.Postgres.Primary.HostName,
			Port:     cfg.Postgres.Primary.Port,
			User:     cfg.Postgres.Primary.User,
			Database: cfg.Postgres.Primary.Database,

			Duration: duration,
			Clients:  clients,
			Scale:    scale,
			Progress: progress,
		}

		if err := runner.Init(opts); err != nil {
			return fmt.Errorf("pgbench init failed: %w", err)
		}

		if err := runner.Run(opts); err != nil {
			return fmt.Errorf("pgbench run failed: %w", err)
		}

		fmt.Println("✅ Benchmark completed successfully.")
		return nil

	case "cloud":
		return fmt.Errorf("benchmark target %q not implemented yet", target)
	default:
		return fmt.Errorf("unsupported target %q (only \"local\" is supported for now)", target)
	}
}

// usage returns the usage string instead of printing+os.Exit.
func usage() string {
	return `Usage:
  telemetryctl <command> <target> [flags]

Commands:
  provision   Provision PostgreSQL resources
  destroy     Destroy PostgreSQL resources
  benchmark   Run pgbench benchmark against PostgreSQL

Targets:
  local       Use local Docker-based PostgreSQL

Flags:
  --config      Path to YAML config file (default: config.yaml)
  --duration    Benchmark duration in seconds (benchmark)
  --clients     Number of concurrent clients (benchmark)
  --scale       pgbench scale factor (benchmark)
  --progress    pgbench progress interval in seconds (benchmark)

Examples:
  telemetryctl provision local --config config.example.yaml
  telemetryctl benchmark local --config config.example.yaml --duration 60 --clients 20 --scale 1 --progress 5
  telemetryctl destroy   local --config config.example.yaml
`
}
