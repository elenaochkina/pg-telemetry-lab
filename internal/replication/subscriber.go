package replication

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

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

// NewSubscriber constructs a Subscriber bound to a database connection
// representing a replica (subscriber) database.
//
// The same Subscriber logic can be reused for any number of replicas by
// providing different DB implementations.
func NewSubscriber(db DB) *Subscriber {
	return &Subscriber{db: db}
}

type SubscriptionSpec struct {
	Name        string
	ConnString  string // points to publisher
	Publication string

	CopyData   bool
	CreateSlot bool
	Enabled    bool
}

// EnsureSubscription creates a subscription if it doesn't already exist.
func (s *Subscriber) EnsureSubscription(ctx context.Context, spec SubscriptionSpec) error {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Publication = strings.TrimSpace(spec.Publication)
	spec.ConnString = strings.TrimSpace(spec.ConnString)

	if spec.Name == "" {
		return fmt.Errorf("subscription name must be set")
	}
	if spec.Publication == "" {
		return fmt.Errorf("publication name must be set")
	}
	if spec.ConnString == "" {
		return fmt.Errorf("publisher connstring must be set")
	}

	exists, err := s.subscriptionExists(ctx, spec.Name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	stmt := fmt.Sprintf(
		"CREATE SUBSCRIPTION %s CONNECTION %s PUBLICATION %s WITH (copy_data = %t, create_slot = %t, enabled = %t)",
		pgx.Identifier{spec.Name}.Sanitize(),
		sanitizeStringLiteral(spec.ConnString),
		pgx.Identifier{spec.Publication}.Sanitize(),
		spec.CopyData,
		spec.CreateSlot,
		spec.Enabled,
	)

	if _, err := s.db.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("create subscription %q: %w", spec.Name, err)
	}
	return nil
}

// sanitizeStringLiteral safely quotes a SQL string literal.
func sanitizeStringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func (s *Subscriber) subscriptionExists(ctx context.Context, name string) (bool, error) {
	var ok bool
	if err := s.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_subscription WHERE subname = $1)
	`, name).Scan(&ok); err != nil {
		return false, fmt.Errorf("check pg_subscription: %w", err)
	}
	return ok, nil
}
