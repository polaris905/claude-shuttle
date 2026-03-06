// Copyright (c) 2026 Cong Li
// Licensed under the MIT License. See LICENSE file in the project root for details.

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anthropic/claude-shuttle/internal/config"
	"github.com/anthropic/claude-shuttle/internal/provider"
	"github.com/anthropic/claude-shuttle/internal/rewriter"
	"github.com/anthropic/claude-shuttle/internal/session"
)

// Version info — injected at build time via ldflags, e.g.:
//
//	go build -ldflags "-X main.version=1.0.0 -X main.buildDate=2026-03-05"
var (
	version   = "0.1.0-alpha"
	buildDate = "dev"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	switch cmd {
	case "config":
		runConfig(os.Args[2:])
	case "push":
		runPush(os.Args[2:])
	case "pull":
		runPull(os.Args[2:])
	case "list":
		runList(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("claude-shuttle %s (built %s)\n", version, buildDate)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf("claude-shuttle %s — Shuttle Claude Code sessions across devices\n", version)
	fmt.Println(`
Commands:
  config    Configure cloud storage settings
  push      Upload a session to cloud storage
  pull      Download a session from cloud storage
  list      List sessions available in cloud storage
  version   Show version information
  help      Show this help message

Setup:
  claude-shuttle config --storage onedrive --remote-path "C:\Users\me\OneDrive\claude-shuttle"

Usage:
  claude-shuttle config                                          Show current config
  claude-shuttle push                                            Select and push a session
  claude-shuttle push -s <session-id>                            Push a specific session
  claude-shuttle list                                            List cloud sessions
  claude-shuttle pull                                            Select and pull a cloud session
  claude-shuttle pull -s <session-id>                            Pull a specific session

Path Mappings:
  When pulling to a different device, claude-shuttle automatically detects file
  paths in the session and asks where they map to locally. You can choose to
  save these mappings so future pulls resolve them automatically.

Copyright (c) 2026 Cong Li
Licensed under the MIT License.`)
}

// --- config command ---

func runConfig(args []string) {
	fs := flag.NewFlagSet("config", flag.ExitOnError)
	storageType := fs.String("storage", "", "Storage type (onedrive)")
	remotePath := fs.String("remote-path", "", "Path to cloud storage folder")
	fs.Parse(args)

	if *storageType == "" && *remotePath == "" {
		// Show current config
		cfg, err := config.Load()
		if err != nil {
			fmt.Println("No config found. Run: claude-shuttle config --storage onedrive --remote-path <path>")
			return
		}
		data, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(data))
		return
	}

	cfg := &config.Config{
		MachineID: config.GetMachineID(),
	}

	// Load existing if available
	if existing, err := config.Load(); err == nil {
		cfg = existing
	}

	if *storageType != "" {
		switch *storageType {
		case "onedrive":
			cfg.StorageType = "onedrive"
		default:
			fmt.Fprintf(os.Stderr, "Unsupported storage type: %s (supported: onedrive)\n", *storageType)
			os.Exit(1)
		}
	}

	if *remotePath != "" {
		if info, err := os.Stat(*remotePath); err != nil {
			fmt.Fprintf(os.Stderr, "Path does not exist: %s\nCreate it first.\n", *remotePath)
			os.Exit(1)
		} else if !info.IsDir() {
			fmt.Fprintf(os.Stderr, "Path is not a directory: %s\n", *remotePath)
			os.Exit(1)
		}
		cfg.OneDrive.ShuttleFolder = *remotePath
	}

	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Config saved.")
	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(data))
}

// --- push command ---

func runPush(args []string) {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	sessionID := fs.String("s", "", "Session ID to push (default: most recent)")
	projectPath := fs.String("p", "", "Project path to filter sessions")
	fs.Parse(args)

	cfg, prov := mustLoadConfigAndProvider()

	// Find the session to push, capturing description and timestamp
	var selected session.HistoryEntry
	if *sessionID != "" {
		selected.SessionID = *sessionID
		// Look up description/timestamp from history
		allSessions, _ := session.FindAllSessions()
		for _, s := range allSessions {
			if s.SessionID == *sessionID {
				selected = s
				break
			}
		}
	} else {
		selected = selectSession(*projectPath)
	}
	sid := selected.SessionID

	fmt.Printf("Collecting session %s...\n", sid[:8])
	bundle, err := session.CollectBundle(sid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error collecting session: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("  Project: %s\n", bundle.ProjectPath)
	if bundle.Slug != "" {
		fmt.Printf("  Slug:    %s\n", bundle.Slug)
	}

	bundleDir, err := session.BundleToDir(bundle)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error bundling session: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(bundleDir)

	// Also bundle history entries for this session
	historyEntries, err := session.HistoryEntriesForSession(sid)
	if err == nil && len(historyEntries) > 0 {
		var lines []string
		for _, e := range historyEntries {
			lines = append(lines, string(e))
		}
		historyData := strings.Join(lines, "\n") + "\n"
		os.WriteFile(filepath.Join(bundleDir, "history-entries.jsonl"), []byte(historyData), 0644)
	}

	// Check if session already exists in cloud
	odProv := prov.(*provider.OneDriveProvider)
	existingSessions, _ := prov.ListSessions()
	for _, s := range existingSessions {
		if s.SessionID == sid {
			fmt.Printf("  WARNING: Session %s already exists in cloud (pushed from %s at %s).\n",
				sid[:8], s.MachineID, s.TransferredAt.Format("2006-01-02 15:04"))
			fmt.Print("  This will overwrite the remote copy. Continue? (y/N): ")
			reader := bufio.NewReader(os.Stdin)
			input, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(input)) != "y" {
				fmt.Println("Aborted.")
				return
			}
			break
		}
	}

	fmt.Println("Copying to cloud storage folder...")
	if err := prov.PushSession(sid, bundleDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error pushing session: %v\n", err)
		os.Exit(1)
	}
	info := provider.SessionInfo{
		SessionID:   sid,
		ProjectPath: bundle.ProjectPath,
		ProjectKey:  bundle.ProjectKey,
		MachineID:   cfg.MachineID,
		Slug:        bundle.Slug,
		Description: selected.Display,
		Timestamp:   selected.Timestamp,
		TransferredAt: time.Now().UTC(),
	}
	if err := odProv.UpdateManifest(info); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to update manifest: %v\n", err)
	}

	fmt.Printf("Session %s copied to cloud storage folder.\n", sid[:8])
	fmt.Println("Note: Your cloud storage client will upload the files in the background.")
	fmt.Println("      Allow a moment for the upload to finish before shutting down.")
}

// --- pull command ---

func runPull(args []string) {
	fs := flag.NewFlagSet("pull", flag.ExitOnError)
	sessionID := fs.String("s", "", "Session ID to pull (default: most recent)")
	fs.Parse(args)

	cfg, prov := mustLoadConfigAndProvider()

	// If no session ID specified, let user pick from cloud sessions
	if *sessionID == "" {
		sessions, err := prov.ListSessions()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
			os.Exit(1)
		}
		if len(sessions) == 0 {
			fmt.Fprintln(os.Stderr, "No sessions available in cloud storage.")
			os.Exit(1)
		}
		// Sort by timestamp descending
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].Timestamp > sessions[j].Timestamp
		})
		selected := selectCloudSession(sessions)
		*sessionID = selected.SessionID
	}

	claudeDir, err := config.ClaudeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Pull session bundle to temp dir
	tmpDir, err := os.MkdirTemp("", "claude-shuttle-pull-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Printf("Pulling session %s...\n", truncate(*sessionID, 8))
	if err := prov.PullSession(*sessionID, tmpDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error pulling session: %v\n", err)
		os.Exit(1)
	}

	// Read metadata
	metaData, err := os.ReadFile(filepath.Join(tmpDir, "metadata.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading metadata: %v\n", err)
		os.Exit(1)
	}
	var meta struct {
		SessionID   string `json:"sessionId"`
		ProjectPath string `json:"projectPath"`
		ProjectKey  string `json:"projectKey"`
		Slug        string `json:"slug"`
	}
	json.Unmarshal(metaData, &meta)

	// --- Smart path detection and mapping ---
	sessionJSONLSrc := filepath.Join(tmpDir, "session.jsonl")

	// Detect all unique path roots in the session JSONL
	fmt.Println("  Scanning session for file paths...")
	detectedPaths, err := rewriter.DetectPaths(sessionJSONLSrc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: path detection failed: %v\n", err)
	}

	// Build the path rewrite map: old path -> new path
	pathRewrites := make(map[string]string)
	reader := bufio.NewReader(os.Stdin)
	newMappings := make(map[string]string) // mappings to offer saving

	// Resolve the primary project path first
	localProjectPath := resolvePathInteractive(meta.ProjectPath, cfg, reader, "project")
	if _, err := os.Stat(localProjectPath); err != nil {
		fmt.Fprintf(os.Stderr, "Project path does not exist: %s\n", localProjectPath)
		os.Exit(1)
	}
	if meta.ProjectPath != localProjectPath {
		pathRewrites[meta.ProjectPath] = localProjectPath
		if cfg.ResolveProjectPath(meta.ProjectPath) == "" {
			newMappings[meta.ProjectPath] = localProjectPath
		}
	}

	// Now resolve any other detected path roots
	if len(detectedPaths) > 0 {
		for _, dp := range detectedPaths {
			// Skip if it's under the project path (already covered)
			if strings.HasPrefix(dp.Original, meta.ProjectPath) {
				continue
			}
			// Skip if already mapped via project path rewrite
			alreadyCovered := false
			for oldP := range pathRewrites {
				if strings.HasPrefix(dp.Original, oldP) {
					alreadyCovered = true
					break
				}
			}
			if alreadyCovered {
				continue
			}

			// Check if this path exists locally (same machine)
			if _, err := os.Stat(dp.Original); err == nil {
				continue // path exists, no rewrite needed
			}

			// Need to resolve this path
			fmt.Printf("\n  Found %d references to paths under: %s\n", dp.Count, dp.Original)
			localPath := resolvePathInteractive(dp.Original, cfg, reader, "path")
			if localPath != "" && localPath != dp.Original {
				pathRewrites[dp.Original] = localPath
				if cfg.ResolveProjectPath(dp.Original) == "" {
					newMappings[dp.Original] = localPath
				}
			}
		}
	}

	// Offer to save new mappings
	if len(newMappings) > 0 {
		fmt.Printf("\n  %d new path mapping(s) discovered.\n", len(newMappings))
		fmt.Print("  Save these mappings for future pulls? (Y/n): ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) != "n" {
			if cfg.PathMappings == nil {
				cfg.PathMappings = make(map[string]string)
			}
			for k, v := range newMappings {
				cfg.PathMappings[k] = v
				fmt.Printf("    Saved: %s -> %s\n", k, v)
			}
			if err := config.Save(cfg); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: failed to save mappings: %v\n", err)
			}
		}
	}

	localProjectKey := session.ProjectKeyFromPath(localProjectPath)
	needsRewrite := len(pathRewrites) > 0

	fmt.Printf("  Project key: %s\n", localProjectKey)
	if needsRewrite {
		fmt.Printf("  Path rewrites: %d mapping(s)\n", len(pathRewrites))
	}

	// Create target directories
	projectDir := filepath.Join(claudeDir, "projects", localProjectKey)
	os.MkdirAll(projectDir, 0755)

	// Check if this session's conversation already exists locally
	sessionJSONLDst := filepath.Join(projectDir, *sessionID+".jsonl")
	if _, err := os.Stat(sessionJSONLDst); err == nil {
		fmt.Println("\n  This session's conversation history already exists locally.")
		fmt.Print("  Overwrite it with the cloud version? (y/N): ")
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) != "y" {
			fmt.Println("Aborted.")
			return
		}
	}
	copyFile(sessionJSONLSrc, sessionJSONLDst)

	// Rewrite all detected paths
	if needsRewrite {
		for oldPath, newPath := range pathRewrites {
			if err := rewriter.RewritePaths(sessionJSONLDst, oldPath, newPath); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: path rewriting failed for %s: %v\n", oldPath, err)
			}
		}
	}

	// Copy session-scoped files (safe to overwrite — they belong to this session only)
	fmt.Println("  Copying session data...")

	// Session data dir (subagents, tool-results)
	sessionDataSrc := filepath.Join(tmpDir, "session-data")
	if info, err := os.Stat(sessionDataSrc); err == nil && info.IsDir() {
		sessionDataDst := filepath.Join(projectDir, *sessionID)
		copyDirContents(sessionDataSrc, sessionDataDst)
	}

	// File history (session-scoped)
	fhSrc := filepath.Join(tmpDir, "file-history")
	if info, err := os.Stat(fhSrc); err == nil && info.IsDir() {
		fhDst := filepath.Join(claudeDir, "file-history", *sessionID)
		copyDirContents(fhSrc, fhDst)
	}

	// Tasks (session-scoped)
	tasksSrc := filepath.Join(tmpDir, "tasks")
	if info, err := os.Stat(tasksSrc); err == nil && info.IsDir() {
		tasksDst := filepath.Join(claudeDir, "tasks", *sessionID)
		copyDirContents(tasksSrc, tasksDst)
	}

	// --- Shared files: these may be used by other sessions, ask before overwriting ---

	// Plan file (could be referenced by other sessions with same slug)
	planSrc := filepath.Join(tmpDir, "plan.md")
	if _, err := os.Stat(planSrc); err == nil && meta.Slug != "" {
		planDst := filepath.Join(claudeDir, "plans", meta.Slug+".md")
		if shouldWriteSharedFile(planDst, reader, "plan file ("+meta.Slug+".md)") {
			copyFile(planSrc, planDst)
		}
	}

	// Memory files (project-scoped, shared across sessions)
	// Strategy: per-file, keep the newer version. New files from cloud are always added.
	memorySrc := filepath.Join(tmpDir, "memory")
	if info, err := os.Stat(memorySrc); err == nil && info.IsDir() {
		memoryDst := filepath.Join(projectDir, "memory")
		os.MkdirAll(memoryDst, 0755)
		mergeMemoryDir(memorySrc, memoryDst, reader)
	}

	// Sessions-index.json — only write if it doesn't exist locally.
	// If the user has already used Claude in this project folder,
	// the existing sessions-index.json is already correct for this machine.
	siDst := filepath.Join(projectDir, "sessions-index.json")
	if _, err := os.Stat(siDst); err != nil {
		// Doesn't exist locally — create it with the correct local path
		siData, _ := json.MarshalIndent(map[string]interface{}{
			"version":      1,
			"entries":      []interface{}{},
			"originalPath": localProjectPath,
		}, "", "  ")
		os.WriteFile(siDst, siData, 0644)
	}

	// Merge history entries
	histSrc := filepath.Join(tmpDir, "history-entries.jsonl")
	if _, err := os.Stat(histSrc); err == nil {
		mergeHistoryEntries(claudeDir, histSrc, pathRewrites)
	}

	fmt.Printf("\nSession %s pulled successfully.\n", truncate(*sessionID, 8))
	fmt.Println("\nWhat was written:")
	fmt.Println("  - Session conversation, subagents, file history, tasks (overwritten)")
	fmt.Println("  - History index (appended, existing entries untouched)")
	fmt.Println("  - Plan file (only if you approved above)")
	fmt.Println("  - Memory files (per-file: new files added, conflicts resolved above)")
	fmt.Println("\nYou can now use '/resume' in Claude Code to continue this session.")
}

// --- list command ---

func runList(args []string) {
	_, prov := mustLoadConfigAndProvider()

	sessions, err := prov.ListSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing sessions: %v\n", err)
		os.Exit(1)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions transferred yet.")
		return
	}

	// Sort by timestamp descending (most recent first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp > sessions[j].Timestamp
	})

	fmt.Println("Transferred sessions (most recent first):\n")
	for i, s := range sessions {
		t := time.UnixMilli(s.Timestamp)
		timeStr := t.Format("2006-01-02 15:04")
		if s.Timestamp == 0 {
			timeStr = s.TransferredAt.Format("2006-01-02 15:04")
		}
		desc := s.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Printf("  [%d] %s  %s  %s\n", i+1, s.SessionID[:8], timeStr, truncate(desc, 50))
		fmt.Printf("      Project: %s  |  Machine: %s\n", truncate(s.ProjectPath, 45), s.MachineID)
	}

	fmt.Printf("\nTotal: %d session(s)\n", len(sessions))
	fmt.Println("\nTo pull a session, run:")
	fmt.Println("  claude-shuttle pull                    (select and pull a session)")
	fmt.Println("  claude-shuttle pull -s <session-id>    (pull a specific session)")

	// Print full IDs for copy-paste
	fmt.Println("\nFull session IDs:")
	for _, s := range sessions {
		fmt.Printf("  %s\n", s.SessionID)
	}
}

// --- helpers ---

// selectSession shows a list of local sessions and lets the user pick one.
func selectSession(projectPath string) session.HistoryEntry {
	sessions, err := session.FindRecentSessions(projectPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding sessions: %v\n", err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "No sessions found.")
		os.Exit(1)
	}

	if len(sessions) == 1 {
		return sessions[0]
	}

	fmt.Println("Recent local sessions:")
	limit := 10
	if len(sessions) < limit {
		limit = len(sessions)
	}
	for i := 0; i < limit; i++ {
		s := sessions[i]
		t := time.UnixMilli(s.Timestamp)
		fmt.Printf("  [%d] %s  %s  %s\n", i+1, s.SessionID[:8], t.Format("2006-01-02 15:04"), truncate(s.Display, 50))
		if s.Project != "" {
			fmt.Printf("      Project: %s\n", truncate(s.Project, 60))
		}
	}
	fmt.Print("\nSelect session number (or Enter for most recent): ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return sessions[0]
	}
	var idx int
	fmt.Sscanf(input, "%d", &idx)
	if idx < 1 || idx > limit {
		fmt.Fprintln(os.Stderr, "Invalid selection.")
		os.Exit(1)
	}
	return sessions[idx-1]
}

// selectCloudSession shows a list of cloud sessions and lets the user pick one.
func selectCloudSession(sessions []provider.SessionInfo) provider.SessionInfo {
	fmt.Println("Available cloud sessions:")
	limit := 10
	if len(sessions) < limit {
		limit = len(sessions)
	}
	for i := 0; i < limit; i++ {
		s := sessions[i]
		t := time.UnixMilli(s.Timestamp)
		timeStr := t.Format("2006-01-02 15:04")
		if s.Timestamp == 0 {
			timeStr = s.TransferredAt.Format("2006-01-02 15:04")
		}
		desc := s.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Printf("  [%d] %s  %s  %s\n", i+1, s.SessionID[:8], timeStr, truncate(desc, 50))
		fmt.Printf("      Project: %s  |  Machine: %s\n", truncate(s.ProjectPath, 45), s.MachineID)
	}

	if len(sessions) == 1 {
		fmt.Print("\nPull this session? (Y/n): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) == "n" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
		return sessions[0]
	}

	fmt.Print("\nSelect session number (or Enter for most recent): ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return sessions[0]
	}
	var idx int
	fmt.Sscanf(input, "%d", &idx)
	if idx < 1 || idx > limit {
		fmt.Fprintln(os.Stderr, "Invalid selection.")
		os.Exit(1)
	}
	return sessions[idx-1]
}

func mustLoadConfigAndProvider() (*config.Config, provider.Provider) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var prov provider.Provider
	switch cfg.StorageType {
	case "onedrive":
		p := provider.NewOneDriveProvider(cfg.OneDrive.ShuttleFolder)
		if err := p.TestConnection(); err != nil {
			fmt.Fprintf(os.Stderr, "Storage connection error: %v\n", err)
			os.Exit(1)
		}
		prov = p
	default:
		fmt.Fprintf(os.Stderr, "Unknown storage type: %s\n", cfg.StorageType)
		os.Exit(1)
	}
	return cfg, prov
}

// resolvePathInteractive resolves a remote path to a local path.
// Priority: saved mapping > path exists locally > interactive prompt.
func resolvePathInteractive(remotePath string, cfg *config.Config, reader *bufio.Reader, label string) string {
	// Check saved mappings
	if mapped := cfg.ResolveProjectPath(remotePath); mapped != "" {
		if _, err := os.Stat(mapped); err == nil {
			fmt.Printf("  Using saved mapping for %s: %s -> %s\n", label, remotePath, mapped)
			return mapped
		}
	}

	// Check if path exists locally
	if _, err := os.Stat(remotePath); err == nil {
		fmt.Printf("  Using existing local %s: %s\n", label, remotePath)
		return remotePath
	}

	// Ask user
	fmt.Printf("  Remote %s path: %s\n", label, remotePath)
	fmt.Printf("  Enter local %s path (or press Enter to skip): ", label)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		if label == "project" {
			fmt.Fprintln(os.Stderr, "Project path is required.")
			os.Exit(1)
		}
		return remotePath // skip rewriting for non-project paths
	}
	return input
}

func mergeHistoryEntries(claudeDir, histSrc string, pathRewrites map[string]string) {
	histDst := filepath.Join(claudeDir, "history.jsonl")

	srcData, err := os.ReadFile(histSrc)
	if err != nil {
		return
	}

	// Read existing session IDs from history
	existingIDs := make(map[string]bool)
	if f, err := os.Open(histDst); err == nil {
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			var entry session.HistoryEntry
			json.Unmarshal(scanner.Bytes(), &entry)
			if entry.SessionID != "" {
				existingIDs[entry.SessionID] = true
			}
		}
		f.Close()
	}

	// Append new entries
	f, err := os.OpenFile(histDst, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(strings.NewReader(string(srcData)))
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	added := 0
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry session.HistoryEntry
		json.Unmarshal([]byte(line), &entry)

		// Skip if this session already has entries in local history
		if existingIDs[entry.SessionID] {
			continue
		}

		for oldP, newP := range pathRewrites {
			oldEscaped := strings.ReplaceAll(oldP, `\`, `\\`)
			newEscaped := strings.ReplaceAll(newP, `\`, `\\`)
			line = strings.ReplaceAll(line, oldEscaped, newEscaped)
		}

		f.WriteString(line + "\n")
		added++
	}
	if added > 0 {
		fmt.Printf("  Added %d history entries.\n", added)
	}
}

// mergeMemoryDir merges cloud memory files into the local memory directory.
// - New files (only in cloud): copied in
// - Files in both: keeps the newer one by modification time, asks user if cloud is newer
// - Local-only files: untouched
func mergeMemoryDir(srcDir, dstDir string, reader *bufio.Reader) {
	srcEntries, err := os.ReadDir(srcDir)
	if err != nil {
		return
	}

	added, skipped, updated := 0, 0, 0
	for _, entry := range srcEntries {
		if entry.IsDir() {
			continue
		}
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		dstInfo, err := os.Stat(dstPath)
		if err != nil {
			// File doesn't exist locally — copy it in
			copyFile(srcPath, dstPath)
			added++
			continue
		}

		// File exists in both — compare modification times
		srcInfo, _ := entry.Info()
		if srcInfo.ModTime().After(dstInfo.ModTime()) {
			// Cloud version is newer
			fmt.Printf("  Memory file '%s': cloud version is newer (%s vs local %s).\n",
				entry.Name(),
				srcInfo.ModTime().Format("2006-01-02 15:04"),
				dstInfo.ModTime().Format("2006-01-02 15:04"))
			fmt.Printf("  Overwrite local with cloud version? (y/N): ")
			input, _ := reader.ReadString('\n')
			if strings.TrimSpace(strings.ToLower(input)) == "y" {
				copyFile(srcPath, dstPath)
				updated++
			} else {
				skipped++
			}
		} else {
			// Local version is same age or newer — keep local
			skipped++
		}
	}

	if added > 0 || updated > 0 || skipped > 0 {
		fmt.Printf("  Memory files: %d added, %d updated, %d kept local version.\n", added, updated, skipped)
	}
}

// shouldWriteSharedFile checks if a shared file/dir exists locally and asks user before overwriting.
// Returns true if the file should be written.
func shouldWriteSharedFile(localPath string, reader *bufio.Reader, description string) bool {
	if _, err := os.Stat(localPath); err != nil {
		return true // doesn't exist locally, safe to write
	}
	fmt.Printf("  Local %s already exists. Overwrite with cloud version? (y/N): ", description)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(input)) == "y"
}

func copyFile(src, dst string) error {
	os.MkdirAll(filepath.Dir(dst), 0755)
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func copyDirContents(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		destPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}
		return copyFile(path, destPath)
	})
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
