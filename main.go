package main

import (
	"fmt"
	"github.com/elenaochkina/pg-telemetry-lab/cmd/telemetryctl"
	"log"
	"os"
)

func main() {
	log.SetFlags(0) // cleaner logs (no timestamps)

	if err := telemetryctl.Run(os.Args[1:]); err != nil {
		// Centralized error + usage printing.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
