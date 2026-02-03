package progress

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// State represents the current parsing progress for resume capability.
type State struct {
	Community    string `json:"community"`
	TotalPosts   int    `json:"total_posts"`
	FetchedCount int    `json:"fetched_count"`
	LastOffset   int    `json:"last_offset"`
	Completed    bool   `json:"completed"`
}

// Load reads the progress state from a file.
// Returns a zero state if the file doesn't exist.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{}, nil
		}
		return nil, fmt.Errorf("read progress file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal progress: %w", err)
	}

	return &state, nil
}

// Save writes the progress state to a file atomically (write to temp, then rename).
func Save(path string, state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal progress: %w", err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write progress file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename progress file: %w", err)
	}

	return nil
}
