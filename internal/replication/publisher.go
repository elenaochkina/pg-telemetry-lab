package replication

import (
	"context"
	"fmt"
	"strings"
)

func (p *Publisher) EnsurePublication(ctx context.Context, pubName string, tables []string) error {
	pubName = strings.TrimSpace(pubName)
	if pubName == "" {
		return fmt.Errorf("publication name must be set")
	}
	if len(tables) == 0 {
		return fmt.Errorf("publication tables must not be empty")
	}

	exists, err := p.publicationExists(ctx, pubName)
	if err != nil {
		return err
	}

	if !exists {
		stmt := fmt.Sprintf(
			"CREATE PUBLICATION %s FOR TABLE %s",
			quoteIdent(pubName),
			strings.Join(quoteTables(tables), ", "),
		)
		if _, err := p.db.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("create publication %q: %w", pubName, err)
		}
		return nil
	}

	// Publication exists: ensure all desired tables are included.
	missing, err := p.missingPublicationTables(ctx, pubName, tables)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}

	stmt := fmt.Sprintf(
		"ALTER PUBLICATION %s ADD TABLE %s",
		quoteIdent(pubName),
		strings.Join(quoteTables(missing), ", "),
	)
	if _, err := p.db.Exec(ctx, stmt); err != nil {
		return fmt.Errorf("alter publication %q add tables: %w", pubName, err)
	}
	return nil
}

func (p *Publisher) publicationExists(ctx context.Context, pubName string) (bool, error) {
	var ok bool
	if err := p.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_publication WHERE pubname = $1)
	`, pubName).Scan(&ok); err != nil {
		return false, fmt.Errorf("check pg_publication: %w", err)
	}
	return ok, nil
}

func (p *Publisher) missingPublicationTables(ctx context.Context, pubName string, desired []string) ([]string, error) {
	rows, err := p.db.Query(ctx, `
		SELECT schemaname || '.' || tablename
		FROM pg_publication_tables
		WHERE pubname = $1
	`, pubName)
	if err != nil {
		return nil, fmt.Errorf("query pg_publication_tables: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]struct{}, len(desired))
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, fmt.Errorf("scan publication table: %w", err)
		}
		existing[t] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate publication tables: %w", err)
	}

	var missing []string
	for _, t := range desired {
		if _, ok := existing[t]; !ok {
			missing = append(missing, t)
		}
	}
	return missing, nil
}

func quoteTables(tables []string) []string {
	out := make([]string, 0, len(tables))
	for _, t := range tables {
		out = append(out, quoteTable(t))
	}
	return out
}
