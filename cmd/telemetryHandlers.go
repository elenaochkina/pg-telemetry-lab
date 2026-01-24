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
	"github.com/elenaochkina/pg-telemetry-lab/internal/util"
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
	writerType := fs.String("writer", "json", "metrics writer type: json, grafana")
	pretty := fs.Bool("pretty", true, "pretty-print JSON output (only for json writer, default true)")
	interval := fs.Duration("interval", 0, "polling interval (e.g., 5s, 1m). If not set, collect once and exit")
	duration := fs.Duration("duration", 0, "how long to collect metrics (e.g., 30s, 1m). If not set, run forever")
	output := fs.String("output", "", "output file path (only for json writer, default: stdout)")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Get password from environment
	password, err := util.GetRequiredEnv("PG_PASSWORD")
	if err != nil {
		return err
	}

	// Create telemetry collector via factory
	collector, err := createTelemetryCollector(cfg, password)
	if err != nil {
		return fmt.Errorf("create telemetry collector: %w", err)
	}

	// Create metrics writer based on type
	var metricsWriter telemetry.MetricsWriter
	switch *writerType {
	case "json":
		metricsWriter, err = createMetricsWriter(*output, *pretty)
	case "grafana":
		metricsWriter, err = createGrafanaWriter(cfg)
	default:
		return fmt.Errorf("unknown writer type: %s (supported: json, grafana)", *writerType)
	}

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

// createMetricsWriter creates a JSON writer based on output path.
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

// createGrafanaWriter creates a Prometheus writer for Grafana Cloud.
func createGrafanaWriter(cfg *config.Config) (telemetry.MetricsWriter, error) {
	// Get required environment variables
	endpoint := os.Getenv("GRAFANA_CLOUD_ENDPOINT")
	if endpoint == "" {
		return nil, fmt.Errorf("GRAFANA_CLOUD_ENDPOINT environment variable not set")
	}

	username := os.Getenv("GRAFANA_CLOUD_USER")
	if username == "" {
		return nil, fmt.Errorf("GRAFANA_CLOUD_USER environment variable not set")
	}

	apiKey := os.Getenv("GRAFANA_CLOUD_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GRAFANA_CLOUD_API_KEY environment variable not set")
	}

	// Create global labels for all metrics
	labels := map[string]string{
		"cluster":     "pg-telemetry-lab",
		"environment": "dev",
		"provider":    cfg.Provider,
	}

	fmt.Printf("📊 Grafana Cloud writer configured (endpoint: %s)\n", endpoint)

	return writer.NewPrometheusWriter(endpoint, username, apiKey, labels), nil
}

// startBackgroundTelemetry starts telemetry collection in a background goroutine.
// Returns a cancel function that should be called to stop collection.
func startBackgroundTelemetry(cfg *config.Config, password string, interval time.Duration, writerType, outputFile string) context.CancelFunc {
	// Create telemetry collector
	collector, err := createTelemetryCollector(cfg, password)
	if err != nil {
		fmt.Printf("Error creating telemetry collector: %v\n", err)
		return func() {} // Return no-op cancel function
	}

	// Create metrics writer based on type
	var metricsWriter telemetry.MetricsWriter
	switch writerType {
	case "json":
		metricsWriter, err = createMetricsWriter(outputFile, false)
	case "grafana":
		metricsWriter, err = createGrafanaWriter(cfg)
	default:
		fmt.Printf("Error: unknown writer type: %s\n", writerType)
		return func() {} // Return no-op cancel function
	}

	if err != nil {
		fmt.Printf("Error creating metrics writer: %v\n", err)
		return func() {} // Return no-op cancel function
	}

	// Create context for telemetry collection
	ctx, cancel := context.WithCancel(context.Background())

	// Start telemetry collection in background
	if writerType == "json" {
		fmt.Printf("📊 Starting telemetry collection (interval=%s, output=%s)\n", interval, outputFile)
	} else {
		fmt.Printf("📊 Starting telemetry collection (interval=%s, writer=%s)\n", interval, writerType)
	}

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
  --writer string     Metrics writer: json, grafana (default "json")
  --pretty bool       Pretty-print JSON output (only for json writer, default true)
  --interval duration Polling interval (e.g., 5s, 1m). If not set, collect once and exit
  --duration duration How long to collect metrics (e.g., 30s, 1m). If not set, run forever
  --output string     Output file path (only for json writer, default: stdout)

Examples:
  # Collect once and print to stdout
  telemetryctl telemetry collect

  # Collect every 5 seconds to file
  telemetryctl telemetry collect --interval 5s --output metrics.jsonl

  # Push to Grafana Cloud every 5 seconds
  telemetryctl telemetry collect --writer grafana --interval 5s

  # Push to Grafana Cloud for 2 minutes
  telemetryctl telemetry collect --writer grafana --interval 5s --duration 2m

Note: Grafana writer requires GRAFANA_CLOUD_ENDPOINT, GRAFANA_CLOUD_USER,
      and GRAFANA_CLOUD_API_KEY environment variables (loaded from .env)
`
}
