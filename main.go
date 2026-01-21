package main

import (
	"fmt"
	"log"
	"os"
	"github.com/elenaochkina/pg-telemetry-lab/cmd"
)

func main() {
	log.SetFlags(0) // cleaner logs (no timestamps)

	if err := cmd.Run(os.Args[1:]); err != nil {
		// Centralized error + usage printing.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
