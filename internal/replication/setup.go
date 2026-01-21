package replication

import (
	"context"
	"fmt"
	"time"

	"github.com/elenaochkina/pg-telemetry-lab/internal/benchmark"
	"github.com/elenaochkina/pg-telemetry-lab/internal/config"
	"github.com/elenaochkina/pg-telemetry-lab/internal/db"
	"github.com/elenaochkina/pg-telemetry-lab/internal/provider"
	"github.com/elenaochkina/pg-telemetry-lab/internal/topology"
)

// SetupOptions contains all inputs needed for replication setup.
// This struct is provider-agnostic - it only depends on interfaces.
type SetupOptions struct {
	Config   *config.Config
	Password string

	// Provider abstraction (can be Docker, AWS, etc.)
	Provider provider.PostgresProvider

	// Benchmark runner (can be Docker, local, remote)
	BenchmarkRunner benchmark.Runner

	// Connection topology (provider gives us these)
	// For host-side connections (from CLI/scripts)
	PrimaryTargetHost  topology.PGTarget
	ReplicaTargetsHost []topology.PGTarget

	// For container/internal connections (from inside the cluster)
	// Docker: uses container DNS names
	// AWS: uses RDS endpoints
	PrimaryTargetInternal  topology.PGTarget
	ReplicaTargetsInternal []topology.PGTarget

	// Publisher connection string (used by replicas to connect to primary)
	PublisherConnInfo string

	// Behavior flags
	InitSchema bool // Run pgbench -i to initialize schema
	Verify     bool // Run verification after setup

	// Context for the entire operation
	Context context.Context
}

// Run orchestrates the full replication setup workflow:
//  1. Provision cluster (if needed)
//  2. Initialize pgbench schema on primary
//  3. Create publication on primary
//  4. Initialize pgbench schema on replicas (DDL not replicated)
//  5. Create subscriptions on replicas
//  6. Wait for replicas to catch up
//  7. Verify replication is working (if requested)
func Run(opts SetupOptions) error {
	ctx := opts.Context
	if ctx == nil {
		return fmt.Errorf("context must not be nil")
	}

	// Step 1: Provision cluster
	fmt.Println("== Provisioning cluster ==")
	if err := opts.Provider.ProvisionPostgres(opts.Config); err != nil {
		return fmt.Errorf("provision cluster: %w", err)
	}
	fmt.Println("✅ Cluster provisioned")

	// Step 2: Connect to primary from host
	primaryConn, err := db.Connect(ctx, opts.PrimaryTargetHost, opts.Password)
	if err != nil {
		return fmt.Errorf("connect to primary: %w", err)
	}
	defer primaryConn.Close(ctx)

	// Step 3: Initialize pgbench schema on primary (if requested)
	if opts.InitSchema {
		fmt.Println("== Initializing pgbench schema on primary ==")
		// Use internal target (container/cluster addressing)
		if err := opts.BenchmarkRunner.Init(benchmark.PgBenchOptions{
			HostName: opts.PrimaryTargetInternal.Host,
			Port:     opts.PrimaryTargetInternal.Port,
			User:     opts.PrimaryTargetInternal.User,
			Database: opts.PrimaryTargetInternal.Database,
			Scale:    1, // Small scale for testing
		}); err != nil {
			return fmt.Errorf("initialize pgbench on primary: %w", err)
		}
		fmt.Println("✅ Initialized pgbench on primary")
	}

	// Step 4: Create publication on primary
	fmt.Println("== Creating publication on primary ==")
	pub := NewPublisher(primaryConn)
	if err := pub.EnsurePublication(ctx, opts.Config.Replication.PublicationName, opts.Config.Replication.Tables); err != nil {
		return fmt.Errorf("create publication: %w", err)
	}
	fmt.Println("✅ Publication created")

	// Step 5: Initialize pgbench schema on replicas (if requested)
	// Logical replication only copies data, not DDL
	if opts.InitSchema {
		fmt.Println("== Initializing pgbench schema on replicas ==")
		for _, rt := range opts.ReplicaTargetsInternal {
			if err := opts.BenchmarkRunner.Init(benchmark.PgBenchOptions{
				HostName: rt.Host,
				Port:     rt.Port,
				User:     rt.User,
				Database: rt.Database,
				Scale:    1,
			}); err != nil {
				return fmt.Errorf("initialize pgbench on %s: %w", rt.Label, err)
			}
			fmt.Printf("✅ Initialized pgbench schema on %s\n", rt.Label)
		}
	}

	// Step 6: Create subscriptions on replicas and wait for catch-up
	fmt.Println("== Creating subscriptions and waiting for catch-up ==")
	for i, rt := range opts.ReplicaTargetsHost {
		subName := opts.Config.Replication.SubscriptionName(i)

		// Connect to replica from host
		replicaConn, err := db.Connect(ctx, rt, opts.Password)
		if err != nil {
			return fmt.Errorf("connect to %s: %w", rt.Label, err)
		}

		sub := NewSubscriber(replicaConn)

		// Create subscription
		if err := sub.EnsureSubscription(ctx, SubscriptionSpec{
			Name:        subName,
			ConnString:  opts.PublisherConnInfo,
			Publication: opts.Config.Replication.PublicationName,
			CopyData:    opts.Config.Replication.CopyData,
			CreateSlot:  opts.Config.Replication.CreateSlot,
			Enabled:     true,
		}); err != nil {
			replicaConn.Close(ctx)
			return fmt.Errorf("create subscription on %s: %w", rt.Label, err)
		}

		// Wait for catch-up
		waitCtx, cancel := context.WithTimeout(ctx, opts.Config.Replication.Verify.Timeout.Duration)
		defer cancel()

		if err := sub.WaitUntilCaughtUp(waitCtx, subName, opts.Config.Replication.Verify.PollInterval.Duration, *opts.Config.Replication.Verify.StrictLSNMatch); err != nil {
			replicaConn.Close(ctx)
			return fmt.Errorf("wait for %s to catch up: %w", rt.Label, err)
		}

		fmt.Printf("✅ %s caught up (%s)\n", rt.Label, subName)
		replicaConn.Close(ctx)
	}

	// Step 7: Verify replication is working (if requested)
	if opts.Verify {
		fmt.Println("== Verifying replication ==")
		if err := verifyReplication(ctx, primaryConn, opts.ReplicaTargetsHost, opts.Password); err != nil {
			return fmt.Errorf("verify replication: %w", err)
		}
		fmt.Println("✅ Replication verified")
	}

	fmt.Println("🎉 Logical replication setup complete")
	return nil
}

// verifyReplication inserts a test row on primary and verifies it appears on replicas.
func verifyReplication(ctx context.Context, primaryConn DB, replicaTargets []topology.PGTarget, password string) error {
	testAccountID := int64(999999)

	// Insert test row on primary (use UPSERT to handle re-runs)
	if _, err := primaryConn.Exec(ctx, `
		INSERT INTO public.pgbench_accounts(aid, bid, abalance, filler)
		VALUES ($1, 1, 1000, 'test account from primary')
		ON CONFLICT (aid) DO UPDATE SET filler = EXCLUDED.filler
	`, testAccountID); err != nil {
		return fmt.Errorf("insert test row: %w", err)
	}

	// Verify on each replica (with retry for async replication)
	for _, rt := range replicaTargets {
		replicaConn, err := db.Connect(ctx, rt, password)
		if err != nil {
			return fmt.Errorf("connect to %s for verification: %w", rt.Label, err)
		}

		var filler string
		deadline := time.Now().Add(10 * time.Second)
		found := false

		for time.Now().Before(deadline) {
			err = replicaConn.QueryRow(ctx, `
				SELECT filler FROM public.pgbench_accounts WHERE aid = $1
			`, testAccountID).Scan(&filler)
			if err == nil {
				found = true
				break
			}
			time.Sleep(200 * time.Millisecond)
		}

		replicaConn.Close(ctx)

		if !found {
			return fmt.Errorf("%s did not receive test row (aid=%d) within 10s", rt.Label, testAccountID)
		}

		fmt.Printf("✅ %s received test row: %q\n", rt.Label, filler)
	}

	return nil
}
