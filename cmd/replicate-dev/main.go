package main

import (
	"context"
	"fmt"
	"os"
	"time"

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
	primaryPool, err := db.Connect(ctx, primaryTarget, pw)
	if err != nil {
		panic(err)
	}
	defer primaryPool.Close()

	// 3) Create a simple table and seed data on primary (so publication has something real)
	fmt.Println("== Creating test table on primary ==")
	if _, err := primaryPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS public.repl_test (
			id  BIGINT PRIMARY KEY,
			msg TEXT NOT NULL
		);
	`); err != nil {
		panic(err)
	}
	// Ensure deterministic seed
	if _, err := primaryPool.Exec(ctx, `TRUNCATE TABLE public.repl_test;`); err != nil {
		panic(err)
	}
	if _, err := primaryPool.Exec(ctx, `INSERT INTO public.repl_test(id, msg) VALUES (1, 'seed');`); err != nil {
		panic(err)
	}

	// 4) Ensure publication on primary for repl_test
	pubName := "repl_pub"
	fmt.Println("== Ensuring publication on primary ==")
	pub := replication.NewPublisher(primaryPool)
	if err := pub.EnsurePublication(ctx, pubName, []string{"public.repl_test"}); err != nil {
		panic(err)
	}

	// 5) Create table schema on all replicas (logical replication only copies data, not DDL)
	fmt.Println("== Creating table schema on replicas ==")
	replicas := dockerpg.ReplicaTargets(cfg)
	for _, rt := range replicas {
		replicaPool, err := db.Connect(ctx, rt, pw)
		if err != nil {
			panic(fmt.Errorf("connect %s: %w", rt.Label, err))
		}
		if _, err := replicaPool.Exec(ctx, `
			CREATE TABLE IF NOT EXISTS public.repl_test (
				id  BIGINT PRIMARY KEY,
				msg TEXT NOT NULL
			);
		`); err != nil {
			replicaPool.Close()
			panic(fmt.Errorf("create table on %s: %w", rt.Label, err))
		}
		replicaPool.Close()
		fmt.Printf("✅ Created table on %s\n", rt.Label)
	}

	// 7) For each replica: connect and ensure subscription
	// IMPORTANT: this conninfo is used FROM INSIDE the replica container, so host must be docker-reachable.
	publisherConnInfo := fmt.Sprintf(
		"host=%s port=5432 dbname=%s user=%s password=%s",
		cfg.Postgres.Primary.HostName,        // e.g. pg-primary
		cfg.Postgres.Primary.Database,        // e.g. pgbench
		cfg.Postgres.Primary.User,            // e.g. postgres
		pw,
	)

	fmt.Println("== Ensuring subscriptions and waiting for catch-up ==")
	for i, rt := range replicas {
		subName := fmt.Sprintf("repl_sub_%d", i+1)

		replicaPool, err := db.Connect(ctx, rt, pw)
		if err != nil {
			panic(fmt.Errorf("connect %s: %w", rt.Label, err))
		}

		sub := replication.NewSubscriber(replicaPool)

		// Ensure subscription exists
		if err := sub.EnsureSubscription(ctx, replication.SubscriptionSpec{
			Name:        subName,
			ConnString:  publisherConnInfo,
			Publication: pubName,
			CopyData:    true,  // for this demo table, copy initial seed row
			CreateSlot:  true,
			Enabled:     true,
		}); err != nil {
			replicaPool.Close()
			panic(fmt.Errorf("ensure subscription on %s: %w", rt.Label, err))
		}

		// Wait until caught up
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		if err := sub.WaitUntilCaughtUp(waitCtx, subName, 500*time.Millisecond, true); err != nil {
			replicaPool.Close()
			panic(fmt.Errorf("wait caught up on %s: %w", rt.Label, err))
		}

		fmt.Printf("✅ %s caught up (%s)\n", rt.Label, subName)
		replicaPool.Close()
	}

	// 8) Insert a new row on primary and verify it appears on replicas
	fmt.Println("== Writing a new row on primary and verifying replicas ==")
	if _, err := primaryPool.Exec(ctx, `INSERT INTO public.repl_test(id, msg) VALUES (2, 'hello from primary');`); err != nil {
		panic(err)
	}

	// Verify on each replica
	for _, rt := range replicas {
		replicaPool, err := db.Connect(ctx, rt, pw)
		if err != nil {
			panic(err)
		}

		var msg string
		// Small retry loop (replication is async)
		ok := false
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			err = replicaPool.QueryRow(ctx, `SELECT msg FROM public.repl_test WHERE id = 2`).Scan(&msg)
			if err == nil {
				ok = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		replicaPool.Close()

		if !ok {
			panic(fmt.Errorf("%s did not receive row id=2 in time (last err: %v)", rt.Label, err))
		}
		fmt.Printf("✅ %s received row id=2: %q\n", rt.Label, msg)
	}

	fmt.Println("🎉 Logical replication is working end-to-end.")
}

