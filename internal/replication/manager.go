package replication

// Publisher owns all logical replication operations that must be executed
// on the *primary* (publisher) database.
//
// This includes responsibilities such as:
//   - creating and validating publications
//   - ensuring the correct tables are included in a publication
//
// Publisher does NOT know:
//   - how the database connection was created
//   - whether the database is running in Docker, cloud, or elsewhere
//
// It depends only on the DB interface, which allows the replication
// package to remain testable and provider-agnostic.
type Publisher struct {
	db DB
}

// Subscriber owns all logical replication operations that must be executed
// on a *replica* (subscriber) database.
//
// This includes responsibilities such as:
//   - creating and validating subscriptions
//   - monitoring replication progress via system catalogs
//   - determining when a subscription has fully caught up
//
// Like Publisher, Subscriber depends only on the DB interface and is
// intentionally unaware of connection details or infrastructure.
type Subscriber struct {
	db DB
}

// NewPublisher constructs a Publisher bound to a database connection
// representing the primary (publisher) database.
//
// The caller is responsible for creating and managing the lifetime of
// the underlying DB (e.g., a *pgxpool.Pool).
func NewPublisher(db DB) *Publisher {
	return &Publisher{db: db}
}

// NewSubscriber constructs a Subscriber bound to a database connection
// representing a replica (subscriber) database.
//
// The same Subscriber logic can be reused for any number of replicas by
// providing different DB implementations.
func NewSubscriber(db DB) *Subscriber {
	return &Subscriber{db: db}
}

