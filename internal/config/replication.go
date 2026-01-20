package config

import (
	"fmt"
	"strings"
	"time"
)

// ReplicationConfig is the top-level `replication:` block in config.yaml.
type ReplicationConfig struct {
	Enabled            bool     `yaml:"enabled"`
	PublicationName    string   `yaml:"publication_name"`
	SubscriptionPrefix string   `yaml:"subscription_prefix"`
	Tables             []string `yaml:"tables"`

	// Subscription options
	CopyData   bool `yaml:"copy_data"`
	CreateSlot bool `yaml:"create_slot"`

	Verify VerifyConfig `yaml:"verify"`
}

// VerifyConfig controls how we verify "caught up".
type VerifyConfig struct {
	PollInterval   Duration `yaml:"poll_interval"`      // e.g. "500ms"
	Timeout        Duration `yaml:"timeout"`            // e.g. "2m"
	StrictLSNMatch *bool    `yaml:"strict_lsn_match"`   // default true if omitted
}

// Duration parses Go duration strings from YAML.
type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(b []byte) error {
	dd, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = dd
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

func (v *VerifyConfig) ApplyDefaults() {
	if v.PollInterval.Duration == 0 {
		v.PollInterval = Duration{Duration: 500 * time.Millisecond}
	}
	if v.Timeout.Duration == 0 {
		v.Timeout = Duration{Duration: 2 * time.Minute}
	}
	if v.StrictLSNMatch == nil {
		t := true
		v.StrictLSNMatch = &t
	}
}

func (r *ReplicationConfig) ApplyDefaults() {
	r.Verify.ApplyDefaults()
}

func (r *ReplicationConfig) Validate(replicasCount int) error {
	if !r.Enabled {
		return nil
	}
	if replicasCount <= 0 {
		return fmt.Errorf("replication.enabled is true but postgres.replicas.count is %d", replicasCount)
	}
	if strings.TrimSpace(r.PublicationName) == "" {
		return fmt.Errorf("replication.publication_name must be set when replication.enabled is true")
	}
	if strings.TrimSpace(r.SubscriptionPrefix) == "" {
		return fmt.Errorf("replication.subscription_prefix must be set when replication.enabled is true")
	}
	if len(r.Tables) == 0 {
		return fmt.Errorf("replication.tables must contain at least one table when replication.enabled is true")
	}
	for i, t := range r.Tables {
		if strings.TrimSpace(t) == "" {
			return fmt.Errorf("replication.tables[%d] is empty", i)
		}
	}
	// Reasonable defaults if user omitted verify block.
	r.ApplyDefaults()
	return nil
}

// SubscriptionName returns the deterministic subscription name for replica i (0-based).
// Example: prefix "pgbench_sub_" -> pgbench_sub_1, pgbench_sub_2, ...
func (r *ReplicationConfig) SubscriptionName(replicaIndex int) string {
	return fmt.Sprintf("%s%d", r.SubscriptionPrefix, replicaIndex+1)
}
