package replication

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

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

// NewPublisher constructs a Publisher bound to a database connection
// representing the primary (publisher) database.
//
// The caller is responsible for creating and managing the lifetime of
// the underlying DB (e.g., a *pgx.Conn).
func NewPublisher(db DB) *Publisher {
	return &Publisher{db: db}
}

// EnsurePublication creates a publication if it doesn't exist, or adds missing tables to an existing one.
//
// Security Note on DDL and String Formatting:
//
// This function uses string formatting (fmt.Sprintf) to build DDL statements because PostgreSQL
// does not support parameterized queries for identifiers (publication names, table names).
// Parameterized queries ($1, $2) only work for VALUES, not for structural elements like table names.
//
// We use pgx.Identifier.Sanitize() which is the official pgx library method for safely quoting
// and escaping SQL identifiers according to PostgreSQL rules. This is safer than custom escaping
// because it's:
//   - Battle-tested across thousands of projects
//   - Maintained by PostgreSQL experts
//   - Handles all edge cases (length limits, encoding, reserved keywords, etc.)
//
// This is safe because identifiers come from validated configuration, not untrusted user input.
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
			pgx.Identifier{pubName}.Sanitize(),
			sanitizeTableList(tables),
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
		pgx.Identifier{pubName}.Sanitize(),
		sanitizeTableList(missing),
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

// sanitizeTableList converts a list of table references to safely quoted SQL identifiers.
//
// Expected input format: "schema.table" (e.g., "public.pgbench_accounts")
// Output: Comma-separated quoted identifiers (e.g., "public"."pgbench_accounts", "public"."pgbench_tellers")
//
// Uses pgx.Identifier to ensure proper PostgreSQL quoting and escaping.
func sanitizeTableList(tables []string) string {
	sanitized := make([]string, 0, len(tables))
	for _, t := range tables {
		sanitized = append(sanitized, sanitizeTableName(t))
	}
	return strings.Join(sanitized, ", ")
}

// sanitizeTableName safely quotes a table reference for use in SQL statements.
//
// Expected input: "schema.table" (e.g., "public.pgbench_accounts")
// Output: Quoted identifier (e.g., "public"."pgbench_accounts")
//
// If input contains a dot, it's treated as schema.table and each part is quoted separately.
// Otherwise, the entire string is quoted as a single identifier.
func sanitizeTableName(tableName string) string {
	parts := strings.Split(tableName, ".")
	if len(parts) == 2 {
		// Quote schema and table separately: schema.table → "schema"."table"
		return pgx.Identifier{parts[0]}.Sanitize() + "." + pgx.Identifier{parts[1]}.Sanitize()
	}
	// Quote as single identifier
	return pgx.Identifier{tableName}.Sanitize()
}
