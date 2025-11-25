package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config describes the structure of config.example.yaml.
type Config struct {
	Version     int    `yaml:"version"`
	Environment string `yaml:"environment"`

	Postgres struct {
		Image   string `yaml:"image"`
		Primary struct {
			Name     string `yaml:"name"`
			Port     int    `yaml:"port"`
			Database string `yaml:"database"`
			User     string `yaml:"user"`
			Password string `yaml:"password"`
		} `yaml:"primary"`

		Replicas struct {
			Count      int    `yaml:"count"`
			BasePort   int    `yaml:"base_port"`
			NamePrefix string `yaml:"name_prefix"`
		} `yaml:"replicas"`
	} `yaml:"postgres"`
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

// Validate performs basic sanity checks on the configuration.
func (c *Config) Validate() error {
	if c.Version == 0 {
		return fmt.Errorf("version must be set (e.g. 1)")
	}
	if c.Environment == "" {
		return fmt.Errorf("environment must be set (e.g. \"local\")")
	}
	if c.Postgres.Image == "" {
		return fmt.Errorf("postgres.image must be set")
	}
	if c.Postgres.Primary.Name == "" {
		return fmt.Errorf("postgres.primary.name must be set")
	}
	if c.Postgres.Primary.Port == 0 {
		return fmt.Errorf("postgres.primary.port must be > 0")
	}
	if c.Postgres.Replicas.Count < 0 {
		return fmt.Errorf("postgres.replicas.count cannot be negative")
	}
	if c.Postgres.Replicas.Count > 0 {
		if c.Postgres.Replicas.BasePort == 0 {
			return fmt.Errorf("postgres.replicas.base_port must be > 0 when replicas.count > 0")
		}
		if c.Postgres.Replicas.NamePrefix == "" {
			return fmt.Errorf("postgres.replicas.name_prefix must be set when replicas.count > 0")
		}
	}
	return nil
}
