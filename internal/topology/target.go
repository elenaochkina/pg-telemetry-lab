package topology

import "fmt"

// PGTarget is provider-agnostic connection info for a Postgres instance.
// No Docker-specific fields required.
type PGTarget struct {
	// Optional human label ("primary", "replica-1") — useful for logs/CLI output.
	Label string

	Host     string
	Port     int
	Database string
	User     string
}

func (t PGTarget) Addr() string {
	return fmt.Sprintf("%s:%d", t.Host, t.Port)
}

