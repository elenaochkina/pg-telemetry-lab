package replication

import "strings"

// quoteIdent safely quotes a SQL identifier (e.g. table, schema, publication name).
//
// It wraps the identifier in double quotes and escapes any embedded quotes
// according to PostgreSQL rules.
//
// This helper is used when generating SQL statements such as:
//   CREATE PUBLICATION "pub_name"
//   ALTER PUBLICATION "pub_name" ADD TABLE "schema"."table"
//
// NOTE:
// - This is intended for controlled inputs coming from configuration.
// - For fully dynamic user input, pgx.Identifier should be preferred.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// quoteLiteral safely quotes a SQL string literal.
//
// It wraps the value in single quotes and escapes embedded single quotes.
// This is primarily used for values such as connection strings:
//
//   CONNECTION 'host=pg-primary port=5432 dbname=pgbench ...'
//
// NOTE:
// - This helper assumes trusted configuration input.
// - It should not be used to quote arbitrary end-user input.
func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, `'`, `''`) + "'"
}

// quoteTable quotes a table reference for use in SQL statements.
//
// Expected input format (recommended):
//   "schema.table"
//
// In this case, both schema and table are quoted individually:
//   "schema"."table"
//
// If the input does not match the expected format, the entire string
// is quoted as a single identifier as a fallback.
//
// This function is used when building publication definitions such as:
//   CREATE PUBLICATION ... FOR TABLE "public"."pgbench_accounts"
func quoteTable(t string) string {
	parts := strings.Split(t, ".")
	if len(parts) == 2 {
		return quoteIdent(parts[0]) + "." + quoteIdent(parts[1])
	}
	return quoteIdent(t)
}

