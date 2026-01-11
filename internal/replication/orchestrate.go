package replication

import (
	"context"
	"fmt"

	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/db"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg"
	"github.com/elenaochkina/pg-telemetry-lab/internal/util"
)

// EnsureLogicalReplication provisions publication on primary and subscriptions on replicas,
// then waits for each replica to catch up.
//
// This is the "circle everything" entry point for local Docker.
func EnsureLogicalReplication(ctx context.Context, cfg *config.Config) error {
	if !cfg.Replication.Enabled {
		return nil
	}
	if cfg.Postgres.Replicas.Count == 0 {
		return fmt.Errorf("replication enabled but replicas.count is 0")
	}

	pw, err := util.GetRequiredEnv("PG_PASSWORD")
	if err != nil {
		return err
	}

	// 1) Connect to primary from host
	primaryTarget := dockerpg.PrimaryTarget(cfg)
	primaryConn, err := db.Connect(ctx, primaryTarget, pw)
	if err != nil {
		return err
	}
	defer primaryConn.Close(ctx)

	// 2) Ensure publication on primary
	pub := NewPublisher(primaryConn)
	if err := pub.EnsurePublication(ctx, cfg.Replication.PublicationName, cfg.Replication.Tables); err != nil {
		return err
	}

	// 3) Build publisher conninfo for subscriptions (used from inside replica containers)
	// For now use same postgres user. Later introduce cfg.Replication.User if desired.
	pubConnInfo := dockerpg.PublisherConnInfo(cfg, cfg.Postgres.Primary.User, pw)

	// 4) For each replica: connect + create subscription + wait caught up
	replicaTargets := dockerpg.ReplicaTargets(cfg)

	// Trust that config has applied defaults via ApplyDefaults()
	poll := cfg.Replication.Verify.PollInterval.Duration
	timeout := cfg.Replication.Verify.Timeout.Duration
	strict := *cfg.Replication.Verify.StrictLSNMatch

	for i, t := range replicaTargets {
		replicaConn, err := db.Connect(ctx, t, pw)
		if err != nil {
			return fmt.Errorf("connect replica %d (%s): %w", i+1, t.Addr(), err)
		}
		func() {
			defer replicaConn.Close(ctx)

			sub := NewSubscriber(replicaConn)
			subName := cfg.Replication.SubscriptionName(i)

			if err := sub.EnsureSubscription(ctx, SubscriptionSpec{
				Name:        subName,
				ConnString:  pubConnInfo, // IMPORTANT: docker-internal conninfo
				Publication: cfg.Replication.PublicationName,
				CopyData:    cfg.Replication.CopyData,
				CreateSlot:  cfg.Replication.CreateSlot,
				Enabled:     true,
			}); err != nil {
				panic(fmt.Errorf("ensure subscription %q: %w", subName, err))
			}

			waitCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			if err := sub.WaitUntilCaughtUp(waitCtx, subName, poll, strict); err != nil {
				panic(fmt.Errorf("wait caught up %q: %w", subName, err))
			}
		}()
	}

	return nil
}
