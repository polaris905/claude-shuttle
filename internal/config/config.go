// Copyright (c) 2026 Cong Li
// Licensed under the MIT License. See LICENSE file in the project root for details.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type OneDriveConfig struct {
	ShuttleFolder string `json:"shuttleFolder"`
}

type Config struct {
	StorageType  string            `json:"storageType"`
	OneDrive     OneDriveConfig    `json:"onedrive,omitempty"`
	MachineID    string            `json:"machineId"`
	// PathMappings maps remote project paths to local paths on this machine.
	// e.g., "C:\\Users\\alice\\repos\\Foo" -> "C:\\Users\\bob\\projects\\Foo"
	PathMappings map[string]string `json:"pathMappings,omitempty"`
}

// ResolveProjectPath looks up a remote project path in the local path mappings.
// Returns the mapped local path if found, or empty string if no mapping exists.
func (c *Config) ResolveProjectPath(remotePath string) string {
	if c.PathMappings == nil {
		return ""
	}
	if local, ok := c.PathMappings[remotePath]; ok {
		return local
	}
	return ""
}

// ClaudeDir returns the path to ~/.claude/
func ClaudeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// ConfigPath returns the path to the shuttle config file.
func ConfigPath() (string, error) {
	claudeDir, err := ClaudeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(claudeDir, "shuttle-config.json"), nil
}

// Load reads the config from disk.
func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no config found — run 'claude-shuttle config' first")
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config to disk.
func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetMachineID returns a machine identifier.
func GetMachineID() string {
	name, err := os.Hostname()
	if err != nil {
		return runtime.GOOS + "-unknown"
	}
	return name
}
