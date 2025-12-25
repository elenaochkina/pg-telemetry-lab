package dockerpg

import (
	"fmt"

	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
)

// PublisherConnInfo returns a libpq-style conninfo string that can be used
// inside CREATE SUBSCRIPTION ... CONNECTION '...'
//
// IMPORTANT: This string is consumed from inside the replica container,
// so we use the primary container name and container port 5432.
func PublisherConnInfo(cfg *config.Config, user, password string) string {
	// Primary is reachable on the docker network by container name and port 5432.
	return fmt.Sprintf(
		"host=%s port=5432 dbname=%s user=%s password=%s",
		cfg.Postgres.Primary.HostName,
		cfg.Postgres.Primary.Database,
		user,
		password,
	)
}
