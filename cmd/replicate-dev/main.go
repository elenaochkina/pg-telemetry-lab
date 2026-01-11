package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/elenaochkina/pg-telemetry-lab/internal/benchmark"
	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/db"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider/dockerpg"
	"github.com/elenaochkina/pg-telemetry-lab/internal/replication"
)

func main() {
	cfg, err := config.Load("local.config.example.yaml") // or config.example.yaml
	if err != nil {
		panic(err)
	}
	if cfg.Postgres.Replicas.Count < 2 {
		panic("set postgres.replicas.count to at least 2 in config")
	}

	pw := os.Getenv("PG_PASSWORD")
	if pw == "" {
		panic("PG_PASSWORD not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1) Provision primary + replicas (Docker)
	fmt.Println("== Provisioning Docker Postgres cluster ==")
	prov := dockerpg.NewDockerPostgresProvider()
	if err := prov.ProvisionPostgres(cfg); err != nil {
		panic(err)
	}
	fmt.Println("✅ Provisioned")

	// 2) Connect to primary from host
	primaryTarget := dockerpg.PrimaryTarget(cfg)
	primaryConn, err := db.Connect(ctx, primaryTarget, pw)
	if err != nil {
		panic(err)
	}
	defer primaryConn.Close(ctx)

	// 3) Initialize pgbench schema on primary
	fmt.Println("== Initializing pgbench schema on primary ==")
	runner := benchmark.NewDockerRunner(cfg.Postgres.Image, cfg.Postgres.Network)

	// pgbench runs inside Docker, so use Docker network addressing
	primaryTargetDocker := dockerpg.PrimaryTargetFromDocker(cfg)
	if err := runner.Init(benchmark.PgBenchOptions{
		HostName: primaryTargetDocker.Host,
		Port:     primaryTargetDocker.Port,
		User:     primaryTargetDocker.User,
		Database: primaryTargetDocker.Database,
		Scale:    1, // Small scale for testing
	}); err != nil {
		panic(fmt.Errorf("initialize pgbench on primary: %w", err))
	}
	fmt.Println("✅ Initialized pgbench on primary")

	// 4) Ensure publication on primary for pgbench tables
	fmt.Println("== Ensuring publication on primary ==")
	pub := replication.NewPublisher(primaryConn)
	if err := pub.EnsurePublication(ctx, cfg.Replication.PublicationName, cfg.Replication.Tables); err != nil {
		panic(err)
	}

	// 5) Initialize pgbench schema on all replicas (logical replication only copies data, not DDL)
	fmt.Println("== Initializing pgbench schema on replicas ==")
	replicas := dockerpg.ReplicaTargets(cfg)
	for i := range replicas {
		// pgbench runs inside Docker, so use Docker network addressing
		replicaTargetDocker := dockerpg.ReplicaTargetFromDocker(cfg, i)
		if err := runner.Init(benchmark.PgBenchOptions{
			HostName: replicaTargetDocker.Host,
			Port:     replicaTargetDocker.Port,
			User:     replicaTargetDocker.User,
			Database: replicaTargetDocker.Database,
			Scale:    1,
		}); err != nil {
			panic(fmt.Errorf("initialize pgbench on %s: %w", replicaTargetDocker.Label, err))
		}
		fmt.Printf("✅ Initialized pgbench schema on %s\n", replicaTargetDocker.Label)
	}

	// 6) For each replica: connect and ensure subscription
	// IMPORTANT: this conninfo is used FROM INSIDE the replica container, so host must be docker-reachable.
	// Use the topology abstraction to get the correct Docker network address.
	publisherConnInfo := dockerpg.PublisherConnInfo(cfg, cfg.Postgres.Primary.User, pw)

	fmt.Println("== Ensuring subscriptions and waiting for catch-up ==")
	for i, rt := range replicas {
		subName := cfg.Replication.SubscriptionName(i)

		replicaConn, err := db.Connect(ctx, rt, pw)
		if err != nil {
			panic(fmt.Errorf("connect %s: %w", rt.Label, err))
		}

		sub := replication.NewSubscriber(replicaConn)

		// Ensure subscription exists
		if err := sub.EnsureSubscription(ctx, replication.SubscriptionSpec{
			Name:        subName,
			ConnString:  publisherConnInfo,
			Publication: cfg.Replication.PublicationName,
			CopyData:    cfg.Replication.CopyData,
			CreateSlot:  cfg.Replication.CreateSlot,
			Enabled:     true,
		}); err != nil {
			replicaConn.Close(ctx)
			panic(fmt.Errorf("ensure subscription on %s: %w", rt.Label, err))
		}

		// Wait until caught up
		waitCtx, cancel := context.WithTimeout(ctx, cfg.Replication.Verify.Timeout.Duration)
		defer cancel()

		if err := sub.WaitUntilCaughtUp(waitCtx, subName, cfg.Replication.Verify.PollInterval.Duration, *cfg.Replication.Verify.StrictLSNMatch); err != nil {
			replicaConn.Close(ctx)
			panic(fmt.Errorf("wait caught up on %s: %w", rt.Label, err))
		}

		fmt.Printf("✅ %s caught up (%s)\n", rt.Label, subName)
		replicaConn.Close(ctx)
	}

	// 7) Insert a new row on primary and verify it appears on replicas
	fmt.Println("== Writing a new row on primary and verifying replicas ==")
	testAccountID := int64(999999)
	if _, err := primaryConn.Exec(ctx, `
		INSERT INTO public.pgbench_accounts(aid, bid, abalance, filler)
		VALUES ($1, 1, 1000, 'test account from primary')
	`, testAccountID); err != nil {
		panic(err)
	}

	// Verify on each replica
	for _, rt := range replicas {
		replicaConn, err := db.Connect(ctx, rt, pw)
		if err != nil {
			panic(err)
		}

		var filler string
		// Small retry loop (replication is async)
		ok := false
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			err = replicaConn.QueryRow(ctx, `
				SELECT filler FROM public.pgbench_accounts WHERE aid = $1
			`, testAccountID).Scan(&filler)
			if err == nil {
				ok = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		replicaConn.Close(ctx)

		if !ok {
			panic(fmt.Errorf("%s did not receive account aid=%d in time (last err: %v)", rt.Label, testAccountID, err))
		}
		fmt.Printf("✅ %s received account aid=%d: %q\n", rt.Label, testAccountID, filler)
	}

	fmt.Println("🎉 Logical replication is working end-to-end with pgbench tables.")
}

