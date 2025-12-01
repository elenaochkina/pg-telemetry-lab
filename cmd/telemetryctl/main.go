package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg"
)

func main() {
	log.SetFlags(0) // cleaner logs (no timestamps)

	if len(os.Args) < 3 {
		usageAndExit("not enough arguments")
	}

	cmd := os.Args[1]    // provision | destroy
	target := os.Args[2] // local (later maybe cloud)

	// Define flags that come after the subcommands.
	fs := flag.NewFlagSet("telemetryctl", flag.ExitOnError)
	var configPath string
	fs.StringVar(&configPath, "config", "config.yaml", "path to config file")
	if err := fs.Parse(os.Args[3:]); err != nil {
		log.Fatalf("parsing flags: %v", err)
	}

	switch cmd {
	case "provision":
		//Load config only for provision
		cfg, err := config.Load(configPath)
		if err != nil {
			log.Fatalf("failed to load config: %v", err)
		}
		handleProvision(target, cfg)
	case "destroy":
		handleDestroy(target)
	default:
		usageAndExit(fmt.Sprintf("unknown command %q", cmd))
	}
}

func handleProvision(target string, cfg *config.Config) {
	switch target {
	case "local":
		provider := dockerpg.NewDockerPostgresProvider()
		if err := provider.ProvisionPostgres(cfg); err != nil {
			log.Fatalf("provisioning local postgres: %v", err)
		}
		fmt.Println("✅ Local PostgreSQL cluster provisioned successfully.")
	case "cloud":
		log.Fatalf("cloud target not implemented yet")
	default:
		log.Fatalf("unsupported target %q (only \"local\" is supported for now)", target)
	}
}

func handleDestroy(target string) {
	switch target {
	case "local":
		provider := dockerpg.NewDockerPostgresProvider()
		if err := provider.DestroyPostgres(); err != nil {
			log.Fatalf("destroying local postgres: %v", err)
		}
		fmt.Println("✅ Local PostgreSQL cluster destroyed (containers removed).")
	case "cloud":
		log.Fatalf("cloud target not implemented yet")
	default:
		log.Fatalf("unsupported target %q (only \"local\" is supported for now)", target)
	}
}

func usageAndExit(msg string) {
	if msg != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n\n", msg)
	}
	fmt.Fprintf(os.Stderr, `Usage:
  telemetryctl <command> <target> [flags]

Commands:
  provision   Provision PostgreSQL resources
  destroy     Destroy PostgreSQL resources

Targets:
  local       Use local Docker-based PostgreSQL

Flags:
  --config    Path to YAML config file (default: config.yaml)

Examples:
  telemetryctl provision local --config config.example.yaml
  telemetryctl destroy   local --config config.example.yaml
`)
	os.Exit(1)
}
