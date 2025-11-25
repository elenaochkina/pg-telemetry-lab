package provider
import ("github.com/elenaochkina/pg-telemetry-lab/internal/config")

type PostresProvider interface {
	ProvisionLocalPostgres(cfg *config.Config) error
	DestroyLocalPostgres(cfg *config.Config) error
}


