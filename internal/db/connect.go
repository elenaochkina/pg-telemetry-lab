package db

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/elenaochkina/pg-telemetry-lab/internal/topology"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context, target topology.PGTarget, password string) (*pgxpool.Pool, error) {
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

	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		return nil, fmt.Errorf("connect (%s): %w", target.Label, err)
	}

	// Wait for DB readiness: docker run returns before Postgres is accepting connections.
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error

	for time.Now().Before(deadline) {
		// Short ping timeout per attempt.
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		lastErr = pool.Ping(pingCtx)
		cancel()

		if lastErr == nil {
			return pool, nil
		}
		time.Sleep(300 * time.Millisecond)
	}

	// Fail fast so errors show up immediately.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping (%s @ %s): %w", target.Label, target.Addr(), err)
	}

	return pool, nil
}

