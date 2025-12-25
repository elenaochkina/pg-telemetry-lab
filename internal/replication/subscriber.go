package replication

import (
	"context"
	"fmt"
	"strings"
)

type SubscriptionSpec struct {
	Name        string
	ConnString  string // points to publisher
	Publication string

	CopyData   bool
	CreateSlot bool
	Enabled    bool
}

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
		quoteIdent(spec.Name),
		quoteLiteral(spec.ConnString),
		quoteIdent(spec.Publication),
		spec.CopyData,
		spec.CreateSlot,
		spec.Enabled,
	)

	if _, err := s.db.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("create subscription %q: %w", spec.Name, err)
	}
	return nil
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
