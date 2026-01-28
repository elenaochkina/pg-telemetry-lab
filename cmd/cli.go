package cmd

import (
	"fmt"

	"github.com/joho/godotenv"
)

// Run is the CLI entry point. Routes commands to appropriate handlers.
// This is a pure router - no business logic here.
func Run(args []string) error {
	// Load .env file for environment variables (e.g., PG_PASSWORD)
	_ = godotenv.Load()

	if len(args) == 0 {
		return fmt.Errorf("command required\n\n%s", usage())
	}

	command := args[0]

	// Route to handlers
	switch command {
	case "provision":
		return handleProvision(args[1:])
	case "destroy":
		return handleDestroy(args[1:])
	case "benchmark":
		return handleBenchmark(args[1:])
	case "replication":
		return handleReplication(args[1:])
	case "telemetry":
		return handleTelemetry(args[1:])
	default:
		return fmt.Errorf("unknown command: %s\n\n%s", command, usage())
	}
}

func usage() string {
	return `telemetryctl - PostgreSQL telemetry and replication management

Usage:
  telemetryctl <command> [flags]

Commands:
  provision     Provision PostgreSQL cluster
  destroy       Destroy PostgreSQL cluster
  benchmark     Run pgbench benchmark
  replication   Manage logical replication
  telemetry     Collect telemetry metrics

Examples:
  telemetryctl provision local
  telemetryctl benchmark local --duration 120
  telemetryctl replication setup
  telemetryctl telemetry collect
  telemetryctl destroy local

For command-specific help:
  telemetryctl <command> --help
`
}
