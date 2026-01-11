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

// ConnInfo returns a libpq-style connection string for use in CREATE SUBSCRIPTION
// or other PostgreSQL connection contexts.
func (t PGTarget) ConnInfo(password string) string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s",
		t.Host,
		t.Port,
		t.Database,
		t.User,
		password,
	)
}
