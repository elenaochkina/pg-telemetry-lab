package cmd

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/elenaochkina/pg-telemetry-lab/internal/benchmark"
	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/util"
)

// handleProvision provisions a PostgreSQL cluster.
// Controller: parses CLI input, uses factory, delegates to provider.
func handleProvision(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("target required (e.g., 'local')\n\nUsage: telemetryctl provision <target> [flags]")
	}

	target := args[0]

	// Parse flags
	fs := flag.NewFlagSet("provision", flag.ContinueOnError)
	configPath := fs.String("config", "configs/local.docker.yaml", "path to config file")

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Validate target matches config
	if target != "local" {
		return fmt.Errorf("unsupported target: %s (only 'local' supported)", target)
	}

	// Use factory to create provider
	provider, err := createProvider(cfg)
	if err != nil {
		return err
	}

	// Delegate to provider (business logic)
	if err := provider.ProvisionPostgres(cfg); err != nil {
		return fmt.Errorf("provision failed: %w", err)
	}

	fmt.Println("✅ PostgreSQL cluster provisioned successfully")
	return nil
}

// handleDestroy destroys a PostgreSQL cluster.
// Controller: parses CLI input, uses factory, delegates to provider.
func handleDestroy(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("target required (e.g., 'local')\n\nUsage: telemetryctl destroy <target>")
	}

	target := args[0]

	// For destroy, we don't need config (state is in .telemetry/)
	// But we need to know which provider to use
	// For now, assume local = docker
	if target != "local" {
		return fmt.Errorf("unsupported target: %s (only 'local' supported)", target)
	}

	// Create provider (hardcoded to docker for now since we don't have config)
	// TODO: Read provider from state file
	provider, err := createProvider(&config.Config{Provider: "docker"})
	if err != nil {
		return err
	}

	// Delegate to provider (business logic)
	if err := provider.DestroyPostgres(); err != nil {
		return fmt.Errorf("destroy failed: %w", err)
	}

	fmt.Println("✅ PostgreSQL cluster destroyed (containers removed)")
	return nil
}

// handleBenchmark runs a pgbench benchmark.
// Controller: parses CLI input, uses factory, delegates to benchmark runner.
func handleBenchmark(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("target required (e.g., 'local')\n\nUsage: telemetryctl benchmark <target> [flags]")
	}

	target := args[0]

	// Parse flags
	fs := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	configPath := fs.String("config", "configs/local.docker.yaml", "path to config file")
	duration := fs.Int("duration", 60, "benchmark duration in seconds")
	clients := fs.Int("clients", 10, "number of concurrent clients")
	scale := fs.Int("scale", 1, "pgbench scale factor (dataset size)")
	progress := fs.Int("progress", 5, "progress interval in seconds (0 = disabled)")

	// Telemetry flags
	collectTelemetry := fs.Bool("collect-telemetry", false, "collect telemetry metrics during benchmark")
	telemetryInterval := fs.Duration("telemetry-interval", 5*time.Second, "telemetry collection interval")
	telemetryWriter := fs.String("telemetry-writer", "json", "telemetry writer: json, grafana")
	telemetryOutput := fs.String("telemetry-output", "", "telemetry output file (only for json writer, default: benchmark-metrics.jsonl)")

	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Validate target
	if target != "local" {
		return fmt.Errorf("unsupported target: %s (only 'local' supported)", target)
	}

	// Get password from environment
	password, err := util.GetRequiredEnv("PG_PASSWORD")
	if err != nil {
		return err
	}

	// Use factory to create benchmark runner
	runner, err := createBenchmarkRunner(cfg)
	if err != nil {
		return err
	}

	// Get primary target (internal, for benchmark container)
	primaryTarget, err := getPrimaryTargetInternal(cfg)
	if err != nil {
		return err
	}

	// Start telemetry collection BEFORE pgbench initialization (if requested)
	// This allows observing metrics during the initialization workload
	var telemetryCancel context.CancelFunc
	if *collectTelemetry {
		// Set default output file if not specified (only for json writer)
		outputFile := *telemetryOutput
		if outputFile == "" && *telemetryWriter == "json" {
			outputFile = "benchmark-metrics.jsonl"
		}

		// Start telemetry in background
		telemetryCancel = startBackgroundTelemetry(password, *telemetryInterval, *telemetryWriter, outputFile)
		defer func() {
			if telemetryCancel != nil {
				fmt.Println("🛑 Stopping telemetry collection...")
				telemetryCancel()
				time.Sleep(500 * time.Millisecond) // Allow graceful shutdown
				fmt.Println("✅ Telemetry collection stopped")
			}
		}()
	}

	// Initialize pgbench schema
	fmt.Println("🔧 Initializing pgbench schema...")
	if err := runner.Init(benchmark.PgBenchOptions{
		HostName: primaryTarget.Host,
		Port:     primaryTarget.Port,
		User:     primaryTarget.User,
		Database: primaryTarget.Database,
		Scale:    *scale,
	}); err != nil {
		return fmt.Errorf("initialize pgbench: %w", err)
	}

	// Run benchmark
	fmt.Println("🚀 Running benchmark...")
	if err := runner.Run(benchmark.PgBenchOptions{
		HostName: primaryTarget.Host,
		Port:     primaryTarget.Port,
		User:     primaryTarget.User,
		Database: primaryTarget.Database,
		Duration: *duration,
		Clients:  *clients,
		Progress: *progress,
	}); err != nil {
		return fmt.Errorf("run benchmark: %w", err)
	}

	fmt.Println("✅ Benchmark completed")
	return nil
}
