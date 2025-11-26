package provider
import ("github.com/elenaochkina/pg-telemetry-lab/internal/config")

type PostgresProvider interface {
	ProvisionPostgres(cfg *config.Config) error
	DestroyPostgres() error
}


