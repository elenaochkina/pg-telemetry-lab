Logical Replication Architecture (Design Overview)

This project separates infrastructure provisioning, connection topology, and database replication logic to keep the system provider-agnostic and easy to extend beyond local Docker.

1. Infrastructure Provisioning (provider layer)

The PostgresProvider interface is responsible only for lifecycle management:

start and stop Postgres instances

handle provider-specific concerns (Docker today, cloud later)

Example: internal/provider/dockerpg spins up a primary and N replicas using Docker, but does not perform any replication SQL.

2. Topology & Addressing (provider-agnostic)

Postgres connection details are represented by a provider-neutral struct:

type PGTarget struct {
	Label    string
	Host     string
	Port     int
	Database string
	User     string
}


Each provider supplies builders that translate its own configuration into PGTarget values:

Docker → localhost + published ports

Cloud providers → endpoints / load balancers

This keeps all higher-level logic independent of Docker.

3. Connection Management (shared DB layer)

A single helper creates real database connections:

db.Connect(ctx, PGTarget, password) → *pgxpool.Pool


This is the only place that knows how to:

build DSNs

configure pooling

verify connectivity

4. Replication Logic (database layer)

The replication package contains pure SQL logic for logical replication:

EnsurePublication (publisher)

EnsureSubscription (subscriber)

WaitUntilCaughtUp (verification via system catalogs)

Replication code depends only on a small DB interface (Exec / Query / QueryRow), which is satisfied by *pgxpool.Pool. This makes the package testable and decoupled from connection details.

5. Why this design

Avoids mixing Docker logic with SQL logic

Makes replication idempotent and verifiable via system catalogs

Enables future providers (AWS, GCP, Kubernetes) without refactoring replication code

Keeps the current local-Docker setup simple and readable

In short:

Providers create databases.
Topology describes how to reach them.
Replication operates purely at the SQL level.