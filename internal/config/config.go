package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config describes the provider-agnostic configuration structure.
// Supports two providers: docker (local development) and aws (production).
type Config struct {
	Version     int    `yaml:"version"`
	Provider    string `yaml:"provider"`    // "docker" or "aws"
	Environment string `yaml:"environment"` // "local", "staging", "production", etc.

	Postgres struct {
		// Docker-specific fields
		Image   string `yaml:"image,omitempty"`
		Network string `yaml:"network,omitempty"`

		// Docker resource limits
		Resources struct {
			PrimaryCPU  float64 `yaml:"primary_cpu,omitempty"`   // CPU limit for primary (e.g., 2.0)
			ReplicaCPU  float64 `yaml:"replica_cpu,omitempty"`   // CPU limit per replica (e.g., 0.5)
			BenchmarkCPU float64 `yaml:"benchmark_cpu,omitempty"` // CPU limit for pgbench container (e.g., 1.0)
		} `yaml:"resources,omitempty"`

		// AWS-specific fields
		Region           string   `yaml:"region,omitempty"`
		InstanceClass    string   `yaml:"instance_class,omitempty"`
		EngineVersion    string   `yaml:"engine_version,omitempty"`
		StorageType      string   `yaml:"storage_type,omitempty"`
		AllocatedStorage int      `yaml:"allocated_storage,omitempty"`
		VpcID            string   `yaml:"vpc_id,omitempty"`
		SubnetGroup      string   `yaml:"subnet_group,omitempty"`
		SecurityGroups   []string `yaml:"security_groups,omitempty"`

		// Shared fields across all providers
		MaxConnections int `yaml:"max_connections,omitempty"` // PostgreSQL max_connections (default: 100)

		Primary struct {
			// Docker uses "name" for container name
			// AWS uses "identifier" for RDS instance identifier
			HostName   string `yaml:"name,omitempty"`       // Docker: container name
			Identifier string `yaml:"identifier,omitempty"` // AWS: RDS identifier

			Port     int    `yaml:"port"`
			Database string `yaml:"database"`
			User     string `yaml:"user"`
			Password string `yaml:"-"` // Never read password from YAML, load from env or secret manager
		} `yaml:"primary"`

		Replicas struct {
			Count int `yaml:"count"`

			// Docker-specific
			BasePort   int    `yaml:"base_port,omitempty"`
			NamePrefix string `yaml:"name_prefix,omitempty"`

			// AWS-specific
			IdentifierPrefix string `yaml:"identifier_prefix,omitempty"`
		} `yaml:"replicas"`
	} `yaml:"postgres"`

	Replication ReplicationConfig `yaml:"replication"`
}

// Load reads a YAML config file from disk and unmarshals it into Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing YAML in %q: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config %q: %w", path, err)
	}

	return &cfg, nil
}

// Validate performs sanity checks on the configuration.
// Validation is provider-aware: docker and aws have different required fields.
func (c *Config) Validate() error {
	// Common validation (all providers)
	if c.Version == 0 {
		return fmt.Errorf("version must be set (e.g. 1)")
	}
	if c.Environment == "" {
		return fmt.Errorf("environment must be set (e.g. \"local\", \"production\")")
	}

	// Provider validation
	validProviders := []string{"docker", "aws"}
	if c.Provider == "" {
		return fmt.Errorf("provider must be set (docker or aws)")
	}
	if !contains(validProviders, c.Provider) {
		return fmt.Errorf("invalid provider %q, must be one of: docker, aws", c.Provider)
	}

	// Provider-specific validation
	if err := c.validateProviderConfig(); err != nil {
		return err
	}

	// Common postgres validation
	if c.Postgres.Primary.Port == 0 {
		return fmt.Errorf("postgres.primary.port must be > 0")
	}
	if c.Postgres.Primary.Database == "" {
		return fmt.Errorf("postgres.primary.database must be set")
	}
	if c.Postgres.Primary.User == "" {
		return fmt.Errorf("postgres.primary.user must be set")
	}
	if c.Postgres.Replicas.Count < 0 {
		return fmt.Errorf("postgres.replicas.count cannot be negative")
	}

	// Replication validation
	if err := c.Replication.Validate(c.Postgres.Replicas.Count); err != nil {
		return err
	}

	return nil
}

// validateProviderConfig validates provider-specific configuration fields.
func (c *Config) validateProviderConfig() error {
	switch c.Provider {
	case "docker":
		return c.validateDockerConfig()
	case "aws":
		return c.validateAWSConfig()
	default:
		return fmt.Errorf("unknown provider: %s", c.Provider)
	}
}

// validateDockerConfig validates Docker-specific configuration.
func (c *Config) validateDockerConfig() error {
	if c.Postgres.Image == "" {
		return fmt.Errorf("postgres.image required for docker provider")
	}
	if c.Postgres.Network == "" {
		return fmt.Errorf("postgres.network required for docker provider")
	}
	if c.Postgres.Primary.HostName == "" {
		return fmt.Errorf("postgres.primary.name required for docker provider (container name)")
	}

	// Validate replica-specific fields if replicas exist
	if c.Postgres.Replicas.Count > 0 {
		if c.Postgres.Replicas.BasePort == 0 {
			return fmt.Errorf("postgres.replicas.base_port must be > 0 when replicas.count > 0 (docker)")
		}
		if c.Postgres.Replicas.NamePrefix == "" {
			return fmt.Errorf("postgres.replicas.name_prefix must be set when replicas.count > 0 (docker)")
		}
	}

	return nil
}

// validateAWSConfig validates AWS-specific configuration.
func (c *Config) validateAWSConfig() error {
	// Future: AWS provider implementation
	// For now, just return an error indicating it's not implemented
	return fmt.Errorf("AWS provider not yet implemented")

	// When implemented, uncomment and add proper validation:
	/*
	if c.Postgres.Region == "" {
		return fmt.Errorf("postgres.region required for aws provider")
	}
	if c.Postgres.InstanceClass == "" {
		return fmt.Errorf("postgres.instance_class required for aws provider")
	}
	if c.Postgres.EngineVersion == "" {
		return fmt.Errorf("postgres.engine_version required for aws provider")
	}
	if c.Postgres.Primary.Identifier == "" {
		return fmt.Errorf("postgres.primary.identifier required for aws provider (RDS instance identifier)")
	}

	if c.Postgres.Replicas.Count > 0 {
		if c.Postgres.Replicas.IdentifierPrefix == "" {
			return fmt.Errorf("postgres.replicas.identifier_prefix must be set when replicas.count > 0 (aws)")
		}
	}

	return nil
	*/
}

// contains checks if a string exists in a slice.
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
