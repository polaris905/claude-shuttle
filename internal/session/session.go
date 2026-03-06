// Copyright (c) 2026 Cong Li
// Licensed under the MIT License. See LICENSE file in the project root for details.

package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anthropic/claude-shuttle/internal/config"
)

// HistoryEntry represents one line in history.jsonl.
type HistoryEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"`
	Project   string `json:"project"`
	SessionID string `json:"sessionId"`
}

// SessionJSONLHeader represents the first user-type line in a session JSONL.
type SessionJSONLHeader struct {
	Type      string `json:"type"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	Slug      string `json:"slug,omitempty"`
	Version   string `json:"version"`
	GitBranch string `json:"gitBranch"`
}

// SessionBundle holds paths to all files that belong to a session.
type SessionBundle struct {
	SessionID  string
	ProjectKey string
	ProjectPath string
	Slug       string

	// Absolute paths to source files/dirs
	SessionJSONL string   // projects/{key}/{id}.jsonl
	SessionDir   string   // projects/{key}/{id}/ (subagents, tool-results)
	FileHistory  string   // file-history/{id}/
	TasksDir     string   // tasks/{id}/
	PlanFile     string   // plans/{slug}.md
	MemoryDir    string   // projects/{key}/memory/
	SessionsIndex string  // projects/{key}/sessions-index.json
}

// projectKeyFromPath converts a project path to a Claude project key.
// e.g., "C:\Users\congl\source\repos\Pivots" -> "C--Users-congl-source-repos-Pivots"
func ProjectKeyFromPath(projectPath string) string {
	// Normalize to forward slashes first
	p := filepath.ToSlash(projectPath)
	// Replace colon with dash: "C:/..." -> "C-/..."
	// Then replace slashes with dashes: "C-/Users/..." -> "C--Users-..."
	// This matches Claude Code's internal format (e.g., "C--Users-congl-source-repos-Foo")
	p = strings.ReplaceAll(p, ":", "-")
	p = strings.ReplaceAll(p, "/", "-")
	return p
}

// FindRecentSessions returns sessions for a project, sorted newest first.
func FindRecentSessions(projectPath string) ([]HistoryEntry, error) {
	claudeDir, err := config.ClaudeDir()
	if err != nil {
		return nil, err
	}
	historyPath := filepath.Join(claudeDir, "history.jsonl")
	f, err := os.Open(historyPath)
	if err != nil {
		return nil, fmt.Errorf("opening history.jsonl: %w", err)
	}
	defer f.Close()

	// Collect unique sessions for the project
	sessionMap := make(map[string]HistoryEntry)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var entry HistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.SessionID == "" {
			continue
		}
		// Match by project path (case-insensitive on Windows)
		if projectPath != "" && !strings.EqualFold(entry.Project, projectPath) {
			continue
		}
		// Keep the latest timestamp per session
		if existing, ok := sessionMap[entry.SessionID]; !ok || entry.Timestamp > existing.Timestamp {
			sessionMap[entry.SessionID] = entry
		}
	}

	var sessions []HistoryEntry
	for _, e := range sessionMap {
		sessions = append(sessions, e)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp > sessions[j].Timestamp
	})
	return sessions, nil
}

// FindAllSessions returns all unique sessions across all projects, sorted newest first.
func FindAllSessions() ([]HistoryEntry, error) {
	return FindRecentSessions("")
}

// CollectBundle gathers all files for a session into a SessionBundle.
func CollectBundle(sessionID string) (*SessionBundle, error) {
	claudeDir, err := config.ClaudeDir()
	if err != nil {
		return nil, err
	}

	// Find the session JSONL by scanning project directories
	projectsDir := filepath.Join(claudeDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, fmt.Errorf("reading projects dir: %w", err)
	}

	bundle := &SessionBundle{SessionID: sessionID}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		jsonlPath := filepath.Join(projectsDir, entry.Name(), sessionID+".jsonl")
		if _, err := os.Stat(jsonlPath); err == nil {
			bundle.ProjectKey = entry.Name()
			bundle.SessionJSONL = jsonlPath
			bundle.MemoryDir = filepath.Join(projectsDir, entry.Name(), "memory")
			bundle.SessionsIndex = filepath.Join(projectsDir, entry.Name(), "sessions-index.json")

			sessionDir := filepath.Join(projectsDir, entry.Name(), sessionID)
			if info, err := os.Stat(sessionDir); err == nil && info.IsDir() {
				bundle.SessionDir = sessionDir
			}
			break
		}
	}

	if bundle.SessionJSONL == "" {
		return nil, fmt.Errorf("session %s not found in any project", sessionID)
	}

	// Extract project path and slug from the JSONL
	bundle.ProjectPath, bundle.Slug = extractSessionMeta(bundle.SessionJSONL)

	// File history
	fhDir := filepath.Join(claudeDir, "file-history", sessionID)
	if info, err := os.Stat(fhDir); err == nil && info.IsDir() {
		bundle.FileHistory = fhDir
	}

	// Tasks
	tasksDir := filepath.Join(claudeDir, "tasks", sessionID)
	if info, err := os.Stat(tasksDir); err == nil && info.IsDir() {
		bundle.TasksDir = tasksDir
	}

	// Plan file (if slug found)
	if bundle.Slug != "" {
		planPath := filepath.Join(claudeDir, "plans", bundle.Slug+".md")
		if _, err := os.Stat(planPath); err == nil {
			bundle.PlanFile = planPath
		}
	}

	return bundle, nil
}

// extractSessionMeta reads the first few lines of a session JSONL to find cwd and slug.
func extractSessionMeta(jsonlPath string) (projectPath, slug string) {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return "", ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	linesRead := 0
	for scanner.Scan() && linesRead < 50 {
		linesRead++
		var header SessionJSONLHeader
		if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
			continue
		}
		if header.CWD != "" && projectPath == "" {
			projectPath = header.CWD
		}
		if header.Slug != "" && slug == "" {
			slug = header.Slug
		}
		if projectPath != "" && slug != "" {
			break
		}
	}
	return
}

// BundleToDir copies all session files into a temporary bundle directory.
func BundleToDir(bundle *SessionBundle) (string, error) {
	tmpDir, err := os.MkdirTemp("", "claude-shuttle-bundle-*")
	if err != nil {
		return "", fmt.Errorf("creating temp dir: %w", err)
	}

	// Copy session JSONL
	if err := copyFileToDir(bundle.SessionJSONL, filepath.Join(tmpDir, "session.jsonl")); err != nil {
		return "", fmt.Errorf("copying session JSONL: %w", err)
	}

	// Copy session directory (subagents, tool-results)
	if bundle.SessionDir != "" {
		if err := copyDirTo(bundle.SessionDir, filepath.Join(tmpDir, "session-data")); err != nil {
			return "", fmt.Errorf("copying session dir: %w", err)
		}
	}

	// Copy file history
	if bundle.FileHistory != "" {
		if err := copyDirTo(bundle.FileHistory, filepath.Join(tmpDir, "file-history")); err != nil {
			return "", fmt.Errorf("copying file history: %w", err)
		}
	}

	// Copy tasks
	if bundle.TasksDir != "" {
		if err := copyDirTo(bundle.TasksDir, filepath.Join(tmpDir, "tasks")); err != nil {
			return "", fmt.Errorf("copying tasks: %w", err)
		}
	}

	// Copy plan file
	if bundle.PlanFile != "" {
		if err := copyFileToDir(bundle.PlanFile, filepath.Join(tmpDir, "plan.md")); err != nil {
			return "", fmt.Errorf("copying plan: %w", err)
		}
	}

	// Copy memory directory
	if bundle.MemoryDir != "" {
		if info, err := os.Stat(bundle.MemoryDir); err == nil && info.IsDir() {
			if err := copyDirTo(bundle.MemoryDir, filepath.Join(tmpDir, "memory")); err != nil {
				return "", fmt.Errorf("copying memory: %w", err)
			}
		}
	}

	// Copy sessions-index.json
	if bundle.SessionsIndex != "" {
		if _, err := os.Stat(bundle.SessionsIndex); err == nil {
			if err := copyFileToDir(bundle.SessionsIndex, filepath.Join(tmpDir, "sessions-index.json")); err != nil {
				return "", fmt.Errorf("copying sessions index: %w", err)
			}
		}
	}

	// Write metadata
	meta := map[string]interface{}{
		"sessionId":   bundle.SessionID,
		"projectPath": bundle.ProjectPath,
		"projectKey":  bundle.ProjectKey,
		"slug":        bundle.Slug,
		"bundledAt":   time.Now().UTC().Format(time.RFC3339),
	}
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(tmpDir, "metadata.json"), metaData, 0644); err != nil {
		return "", fmt.Errorf("writing metadata: %w", err)
	}

	return tmpDir, nil
}

// HistoryEntriesForSession returns history.jsonl entries matching a session ID.
func HistoryEntriesForSession(sessionID string) ([]json.RawMessage, error) {
	claudeDir, err := config.ClaudeDir()
	if err != nil {
		return nil, err
	}
	historyPath := filepath.Join(claudeDir, "history.jsonl")
	f, err := os.Open(historyPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []json.RawMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var entry HistoryEntry
		line := scanner.Bytes()
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.SessionID == sessionID {
			cp := make([]byte, len(line))
			copy(cp, line)
			entries = append(entries, cp)
		}
	}
	return entries, nil
}

func copyFileToDir(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func copyDirTo(src, dst string) error {
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
		return copyFileToDir(path, destPath)
	})
}
