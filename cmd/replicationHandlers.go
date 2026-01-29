package cmd

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/replication"
	"github.com/elenaochkina/pg-telemetry-lab/internal/util"
)

// handleReplication routes replication subcommands to appropriate handlers.
// Controller: routes to specific replication handlers.
func handleReplication(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("replication subcommand required\n\n%s", replicationUsage())
	}

	subcommand := args[0]

	switch subcommand {
	case "setup":
		return handleReplicationSetup(args[1:])
	default:
		return fmt.Errorf("unknown replication subcommand: %s\n\n%s", subcommand, replicationUsage())
	}
}

// handleReplicationSetup sets up logical replication.
// Controller: parses CLI input, uses factory, delegates to replication service.
func handleReplicationSetup(args []string) error {
	// Parse flags
	fs := flag.NewFlagSet("replication setup", flag.ContinueOnError)
	configPath := fs.String("config", "configs/local.docker.yaml", "path to config file")
	initSchema := fs.Bool("init-schema", true, "initialize pgbench schema")
	noInitSchema := fs.Bool("no-init-schema", false, "skip schema initialization")
	verify := fs.Bool("verify", true, "verify replication after setup")
	noVerify := fs.Bool("no-verify", false, "skip verification")
	timeout := fs.Duration("timeout", 5*time.Minute, "overall operation timeout")

	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parsing flags: %w", err)
	}

	// Handle boolean flag overrides
	if *noInitSchema {
		*initSchema = false
	}
	if *noVerify {
		*verify = false
	}

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Get password
	password, err := util.GetPassword()
	if err != nil {
		return err
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Use factory to create replication setup options
	setupOpts, err := createReplicationSetup(cfg, password, ctx, *initSchema, *verify)
	if err != nil {
		return err
	}

	// Delegate to replication service (business logic)
	// This is where the actual work happens - replication.Run() orchestrates the workflow
	return replication.Run(setupOpts)
}

func replicationUsage() string {
	return `Usage: telemetryctl replication <subcommand> [flags]

Subcommands:
  setup       Setup logical replication (publication + subscriptions)

Flags:
  --config           Path to config file (default: configs/local.docker.yaml)
  --init-schema      Initialize pgbench schema (default: true)
  --no-init-schema   Skip schema initialization
  --verify           Verify replication after setup (default: true)
  --no-verify        Skip verification
  --timeout          Overall operation timeout (default: 5m)

Examples:
  # Full setup with defaults
  telemetryctl replication setup

  # Use custom config
  telemetryctl replication setup --config configs/production.aws.yaml

  # Skip schema initialization
  telemetryctl replication setup --no-init-schema

  # Skip verification
  telemetryctl replication setup --no-verify

  # Custom timeout
  telemetryctl replication setup --timeout 10m
`
}
