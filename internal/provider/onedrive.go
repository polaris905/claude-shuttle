// Copyright (c) 2026 Cong Li
// Licensed under the MIT License. See LICENSE file in the project root for details.

package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// OneDriveProvider transfers sessions via a local OneDrive folder.
type OneDriveProvider struct {
	ShuttleFolder string
}

func NewOneDriveProvider(shuttleFolder string) *OneDriveProvider {
	return &OneDriveProvider{ShuttleFolder: shuttleFolder}
}

func (p *OneDriveProvider) TestConnection() error {
	info, err := os.Stat(p.ShuttleFolder)
	if err != nil {
		return fmt.Errorf("shuttle folder not accessible: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("shuttle folder path is not a directory: %s", p.ShuttleFolder)
	}
	return nil
}

func (p *OneDriveProvider) sessionsDir() string {
	return filepath.Join(p.ShuttleFolder, "sessions")
}

func (p *OneDriveProvider) manifestPath() string {
	return filepath.Join(p.ShuttleFolder, "shuttle-manifest.json")
}

func (p *OneDriveProvider) PushSession(sessionID string, bundleDir string) error {
	destDir := filepath.Join(p.sessionsDir(), sessionID)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating session dir: %w", err)
	}
	if err := copyDirContents(bundleDir, destDir); err != nil {
		return fmt.Errorf("copying session bundle: %w", err)
	}
	return nil
}

func (p *OneDriveProvider) PullSession(sessionID string, destDir string) error {
	srcDir := filepath.Join(p.sessionsDir(), sessionID)
	if _, err := os.Stat(srcDir); err != nil {
		return fmt.Errorf("session %s not found in cloud: %w", sessionID, err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating dest dir: %w", err)
	}
	if err := copyDirContents(srcDir, destDir); err != nil {
		return fmt.Errorf("pulling session: %w", err)
	}
	return nil
}

func (p *OneDriveProvider) ListSessions() ([]SessionInfo, error) {
	data, err := os.ReadFile(p.manifestPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading manifest: %w", err)
	}
	var sessions []SessionInfo
	if err := json.Unmarshal(data, &sessions); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	return sessions, nil
}

func (p *OneDriveProvider) RemoveSession(sessionID string) error {
	dir := filepath.Join(p.sessionsDir(), sessionID)
	return os.RemoveAll(dir)
}

// UpdateManifest adds or updates a session entry in the manifest.
func (p *OneDriveProvider) UpdateManifest(info SessionInfo) error {
	sessions, _ := p.ListSessions()

	// Replace existing or append
	found := false
	for i, s := range sessions {
		if s.SessionID == info.SessionID {
			sessions[i] = info
			found = true
			break
		}
	}
	if !found {
		sessions = append(sessions, info)
	}

	data, err := json.MarshalIndent(sessions, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.ShuttleFolder, 0755); err != nil {
		return err
	}
	return os.WriteFile(p.manifestPath(), data, 0644)
}

// RemoveFromManifest removes a session entry from the manifest.
func (p *OneDriveProvider) RemoveFromManifest(sessionID string) error {
	sessions, _ := p.ListSessions()
	var filtered []SessionInfo
	for _, s := range sessions {
		if s.SessionID != sessionID {
			filtered = append(filtered, s)
		}
	}
	data, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p.manifestPath(), data, 0644)
}

// copyDirContents recursively copies contents of src into dst.
func copyDirContents(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}
		return copyFile(path, destPath)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
