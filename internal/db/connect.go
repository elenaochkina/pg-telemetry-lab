package db

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/elenaochkina/pg-telemetry-lab/internal/topology"
	"github.com/jackc/pgx/v5"
)

func Connect(ctx context.Context, target topology.PGTarget, password string) (*pgx.Conn, error) {
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(target.User, password),
		Host:   target.Addr(),
		Path:   "/" + target.Database,
	}

	// Force local/dev behavior: no TLS.
	// (You can later make this configurable per provider/target.)
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()

	// Wait for DB readiness: docker run returns before Postgres is accepting connections.
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	var conn *pgx.Conn

	for time.Now().Before(deadline) {
		// Short connect timeout per attempt.
		connectCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		conn, lastErr = pgx.Connect(connectCtx, u.String())
		cancel()

		if lastErr == nil {
			// Successfully connected, verify with a ping
			pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			lastErr = conn.Ping(pingCtx)
			cancel()

			if lastErr == nil {
				return conn, nil
			}
			// Ping failed, close and retry
			conn.Close(ctx)
		}
		time.Sleep(300 * time.Millisecond)
	}

	// Final attempt without retry
	conn, err := pgx.Connect(ctx, u.String())
	if err != nil {
		return nil, fmt.Errorf("connect (%s @ %s): %w", target.Label, target.Addr(), err)
	}

	// Verify connection
	if err := conn.Ping(ctx); err != nil {
		conn.Close(ctx)
		return nil, fmt.Errorf("ping (%s @ %s): %w", target.Label, target.Addr(), err)
	}

	return conn, nil
}

