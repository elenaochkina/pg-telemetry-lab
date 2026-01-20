package dockerpg

import (
	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
)

// PublisherConnInfo returns a libpq-style conninfo string that can be used
// inside CREATE SUBSCRIPTION ... CONNECTION '...'
//
// IMPORTANT: This string is consumed from inside the replica container,
// so we use the primary container name and container port 5432.
func PublisherConnInfo(cfg *config.Config, user, password string) string {
	// Use topology abstraction to get the Docker network address for the primary.
	target := PrimaryTargetFromDocker(cfg)
	return target.ConnInfo(password)
}
