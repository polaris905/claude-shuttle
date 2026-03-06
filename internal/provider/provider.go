// Copyright (c) 2026 Cong Li
// Licensed under the MIT License. See LICENSE file in the project root for details.

package provider

import "time"

// SessionInfo holds metadata about a shuttled session.
type SessionInfo struct {
	SessionID   string    `json:"sessionId"`
	ProjectPath string    `json:"projectPath"`
	ProjectKey  string    `json:"projectKey"`
	MachineID   string    `json:"machineId"`
	Slug        string    `json:"slug,omitempty"`
	Description string    `json:"description,omitempty"`
	Timestamp   int64     `json:"timestamp"`
	TransferredAt time.Time `json:"transferredAt"`
}

// Provider defines the interface for cloud storage backends.
type Provider interface {
	// TestConnection verifies the storage backend is accessible.
	TestConnection() error

	// PushSession uploads a session bundle directory to the cloud.
	PushSession(sessionID string, bundleDir string) error

	// PullSession downloads a session bundle from the cloud to destDir.
	PullSession(sessionID string, destDir string) error

	// ListSessions returns all sessions available in the cloud.
	ListSessions() ([]SessionInfo, error)

	// RemoveSession deletes a session from the cloud.
	RemoveSession(sessionID string) error
}
