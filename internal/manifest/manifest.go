// Copyright (c) 2026 Cong Li
// Licensed under the MIT License. See LICENSE file in the project root for details.

package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/anthropic/claude-shuttle/internal/provider"
)

// Load reads the shuttle manifest from a directory.
func Load(dir string) ([]provider.SessionInfo, error) {
	path := filepath.Join(dir, "shuttle-manifest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sessions []provider.SessionInfo
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, err
	}
	return sessions, nil
}

// Save writes the shuttle manifest to a directory.
func Save(dir string, sessions []provider.SessionInfo) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "shuttle-manifest.json")
	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
