package cmd

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/telemetry"
	"github.com/elenaochkina/pg-telemetry-lab/internal/telemetry/writer"
)

// handleTelemetry routes telemetry subcommands to appropriate handlers.
// Controller: routes to specific telemetry handlers.
func handleTelemetry(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("telemetry subcommand required\n\n%s", telemetryUsage())
	}

	subcommand := args[0]

	switch subcommand {
	case "collect":
		return handleTelemetryCollect(args[1:])
	default:
		return fmt.Errorf("unknown telemetry subcommand: %s\n\n%s", subcommand, telemetryUsage())
	}
}

// handleTelemetryCollect collects telemetry metrics and outputs to stdout or file.
// Supports both single-shot collection and continuous polling.
// Controller: parses CLI input, uses factory, delegates to telemetry collector.
func handleTelemetryCollect(args []string) error {
	// Parse flags
	fs := flag.NewFlagSet("telemetry collect", flag.ContinueOnError)
	configPath := fs.String("config", "configs/local.docker.yaml", "path to config file")
	pretty := fs.Bool("pretty", true, "pretty-print JSON output (false for JSONL format)")
	interval := fs.Duration("interval", 0, "polling interval (e.g., 5s, 1m). If not set, collect once and exit")
	duration := fs.Duration("duration", 0, "how long to collect metrics (e.g., 30s, 1m). If not set, run forever")
	output := fs.String("output", "", "output file path (default: stdout)")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Get password from environment
	password, err := getPassword()
	if err != nil {
		return err
	}

	// Create telemetry collector via factory
	collector, err := createTelemetryCollector(cfg, password)
	if err != nil {
		return fmt.Errorf("create telemetry collector: %w", err)
	}

	// Create metrics writer (stdout or file)
	metricsWriter, err := createMetricsWriter(*output, *pretty)
	if err != nil {
		return fmt.Errorf("create metrics writer: %w", err)
	}
	defer metricsWriter.Close()

	// Single-shot collection if no interval specified
	if *interval == 0 {
		return collectOnce(collector, metricsWriter)
	}

	// Continuous polling
	return collectContinuously(collector, metricsWriter, *interval, *duration)
}

// collectOnce collects metrics once and exits.
func collectOnce(collector telemetry.Collector, writer telemetry.MetricsWriter) error {
	ctx := context.Background()
	metrics, err := collector.CollectReplicationLag(ctx)
	if err != nil {
		return fmt.Errorf("collecting replication metrics: %w", err)
	}

	if err := writer.Write(metrics); err != nil {
		return fmt.Errorf("writing metrics: %w", err)
	}

	return nil
}

// collectContinuously polls metrics at regular intervals.
func collectContinuously(collector telemetry.Collector, writer telemetry.MetricsWriter, interval, duration time.Duration) error {
	// Create context with optional timeout
	ctx := context.Background()
	if duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, duration)
		defer cancel()
		fmt.Printf("📊 Starting telemetry collection (interval=%s, duration=%s)\n", interval, duration)
	} else {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		fmt.Printf("📊 Starting telemetry collection (interval=%s, press Ctrl+C to stop)\n", interval)

		// Handle graceful shutdown on Ctrl+C
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Println("\n🛑 Received interrupt signal, stopping collection...")
			cancel()
		}()
	}

	// Start polling (this blocks until context is cancelled)
	if err := collector.StartPolling(ctx, interval, writer); err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			fmt.Println("✅ Telemetry collection stopped")
			return nil
		}
		return fmt.Errorf("polling metrics: %w", err)
	}

	return nil
}

// createMetricsWriter creates a writer based on output path.
func createMetricsWriter(outputPath string, pretty bool) (telemetry.MetricsWriter, error) {
	// Default to stdout
	if outputPath == "" {
		return writer.NewJSONWriter(os.Stdout, pretty), nil
	}

	// Open file in append mode
	file, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening output file %q: %w", outputPath, err)
	}

	// For file output, always use JSONL format (not pretty)
	return writer.NewJSONWriter(file, false), nil
}

// startBackgroundTelemetry starts telemetry collection in a background goroutine.
// Returns a cancel function that should be called to stop collection.
func startBackgroundTelemetry(cfg *config.Config, password string, interval time.Duration, outputFile string) context.CancelFunc {
	// Create telemetry collector
	collector, err := createTelemetryCollector(cfg, password)
	if err != nil {
		fmt.Printf("Error creating telemetry collector: %v\n", err)
		return func() {} // Return no-op cancel function
	}

	// Create metrics writer
	metricsWriter, err := createMetricsWriter(outputFile, false)
	if err != nil {
		fmt.Printf("Error creating metrics writer: %v\n", err)
		return func() {} // Return no-op cancel function
	}

	// Create context for telemetry collection
	ctx, cancel := context.WithCancel(context.Background())

	// Start telemetry collection in background
	fmt.Printf("📊 Starting telemetry collection (interval=%s, output=%s)\n", interval, outputFile)
	go func() {
		defer metricsWriter.Close()

		if err := collector.StartPolling(ctx, interval, metricsWriter); err != nil {
			if err != context.Canceled {
				fmt.Printf("Telemetry collection error: %v\n", err)
			}
		}
	}()

	return cancel
}

func telemetryUsage() string {
	return `Telemetry subcommands:
  collect    Collect replication lag metrics

Usage:
  telemetryctl telemetry collect [flags]

Flags:
  --config string     Path to config file (default "configs/local.docker.yaml")
  --pretty bool       Pretty-print JSON output, false for JSONL format (default true)
  --interval duration Polling interval (e.g., 5s, 1m). If not set, collect once and exit
  --duration duration How long to collect metrics (e.g., 30s, 1m). If not set, run forever
  --output string     Output file path (default: stdout)

Examples:
  # Collect metrics once and print to stdout (pretty JSON)
  telemetryctl telemetry collect

  # Collect metrics in JSONL format
  telemetryctl telemetry collect --pretty=false

  # Collect metrics every 5 seconds continuously (press Ctrl+C to stop)
  telemetryctl telemetry collect --interval 5s

  # Collect metrics every 5 seconds for 30 seconds
  telemetryctl telemetry collect --interval 5s --duration 30s

  # Collect continuously and write to file (JSONL format)
  telemetryctl telemetry collect --interval 5s --output metrics.jsonl

  # Collect for 1 minute and write to file
  telemetryctl telemetry collect --interval 5s --duration 1m --output metrics.jsonl
`
}
