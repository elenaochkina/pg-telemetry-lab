package dockerpg

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const LocalStatePath = ".telemetry/local-state.json"

type LocalState struct {
	PrimaryContainer  string   `json:"primary_container"`
	ReplicaContainers []string `json:"replica_containers"`
	Image             string   `json:"image"`
	CreatedAt         string   `json:"created_at"`
}

// SaveLocalState writes state metadata to disk.
func SaveLocalState(state LocalState) error {
	// Ensure folder exists
	if err := os.MkdirAll(".telemetry", 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	// Auto-fill timestamp if empty
	if state.CreatedAt == "" {
		state.CreatedAt = time.Now().Format(time.RFC3339)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	return os.WriteFile(LocalStatePath, data, 0644)
}

// LoadLocalState reads state metadata from disk.
func LoadLocalState() (*LocalState, error) {
	data, err := os.ReadFile(LocalStatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no local state found, provision first")
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var state LocalState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	return &state, nil
}
