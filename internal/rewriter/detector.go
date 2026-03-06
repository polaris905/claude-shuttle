// Copyright (c) 2026 Cong Li
// Licensed under the MIT License. See LICENSE file in the project root for details.

package rewriter

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Regex for extracting paths from JSON string values
var (
	// Windows path: C:\Users\... or C:\\Users\\...
	winPathRe = regexp.MustCompile(`^([A-Za-z]):[/\\]`)
	// Unix absolute path: /home/..., /Users/...
	unixPathRe = regexp.MustCompile(`^/(?:home|Users|opt|var|root)/`)
)

// DetectedPath represents a unique path root found in the JSONL.
type DetectedPath struct {
	Original string
	Count    int
}

// DetectPaths scans a JSONL file and returns unique path roots found
// in structured metadata fields (cwd, project, file_path, path arguments).
// It does NOT scan freeform text content to avoid false positives.
// Paths that don't share a user home directory with the session's cwd are
// filtered out (they're likely example paths from conversation content).
func DetectPaths(jsonlPath string) ([]DetectedPath, error) {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	pathSet := make(map[string]int)
	cwdSet := make(map[string]bool) // collect actual cwd values to validate against

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 10*1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		extractStructuredPaths(line, pathSet)

		// Collect cwd values as ground truth
		var entry map[string]json.RawMessage
		if json.Unmarshal(line, &entry) == nil {
			if raw, ok := entry["cwd"]; ok {
				var cwd string
				if json.Unmarshal(raw, &cwd) == nil && cwd != "" {
					cwdSet[normalizePath(cwd)] = true
				}
			}
		}
	}

	if len(pathSet) == 0 {
		return nil, nil
	}

	// Filter: only keep paths whose user home prefix matches a known cwd.
	// This eliminates example paths like C:\Users\alice\... when the real user is C:\Users\congl\...
	if len(cwdSet) > 0 {
		homePrefixes := extractHomePrefixes(cwdSet)
		filtered := make(map[string]int)
		for p, count := range pathSet {
			for _, prefix := range homePrefixes {
				if strings.HasPrefix(strings.ToLower(p), strings.ToLower(prefix)) {
					filtered[p] = count
					break
				}
			}
		}
		pathSet = filtered
	}

	if len(pathSet) == 0 {
		return nil, nil
	}

	return findRoots(pathSet), nil
}

// extractHomePrefixes gets user home directory prefixes from cwd values.
// e.g., from "C:\Users\congl\source\repos\Foo" extracts "C:\Users\congl"
// e.g., from "/home/congl/repos/Foo" extracts "/home/congl"
func extractHomePrefixes(cwdSet map[string]bool) []string {
	prefixes := make(map[string]bool)
	for cwd := range cwdSet {
		normalized := filepath.ToSlash(cwd)
		parts := strings.Split(normalized, "/")
		// Windows: C:/Users/congl/... -> take first 3 parts (C:, Users, congl)
		// Unix: /home/congl/... -> take first 3 parts ("", home, congl)
		if len(parts) >= 3 {
			prefix := filepath.FromSlash(strings.Join(parts[:3], "/"))
			prefixes[prefix] = true
		}
	}
	var result []string
	for p := range prefixes {
		result = append(result, p)
	}
	return result
}

// extractStructuredPaths parses a JSONL line and extracts paths from
// specific fields known to contain file paths.
func extractStructuredPaths(line []byte, pathSet map[string]int) {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(line, &entry); err != nil {
		return
	}

	// Direct metadata fields: cwd, project
	for _, field := range []string{"cwd", "project"} {
		if raw, ok := entry[field]; ok {
			var val string
			if json.Unmarshal(raw, &val) == nil && isAbsolutePath(val) {
				pathSet[normalizePath(val)]++
			}
		}
	}

	// Recurse into message.content to find tool_use blocks with file paths
	if raw, ok := entry["message"]; ok {
		var msg struct {
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(raw, &msg) == nil && msg.Content != nil {
			extractPathsFromContent(msg.Content, pathSet)
		}
	}
}

// extractPathsFromContent looks for tool_use content blocks and extracts
// file_path/path arguments from tool inputs.
func extractPathsFromContent(content json.RawMessage, pathSet map[string]int) {
	// Content can be a string or array of content blocks
	var blocks []json.RawMessage
	if err := json.Unmarshal(content, &blocks); err != nil {
		return // string content, skip
	}

	for _, block := range blocks {
		var cb struct {
			Type  string          `json:"type"`
			Input json.RawMessage `json:"input"`
		}
		if json.Unmarshal(block, &cb) != nil || cb.Type != "tool_use" {
			continue
		}
		if cb.Input == nil {
			continue
		}

		// Extract known path fields from tool inputs
		var input map[string]json.RawMessage
		if json.Unmarshal(cb.Input, &input) != nil {
			continue
		}
		for _, field := range []string{"file_path", "path", "command"} {
			if raw, ok := input[field]; ok {
				var val string
				if json.Unmarshal(raw, &val) == nil {
					if field == "command" {
						// Extract paths from shell commands
						extractPathsFromCommand(val, pathSet)
					} else if isAbsolutePath(val) {
						pathSet[normalizePath(val)]++
					}
				}
			}
		}
	}
}

// extractPathsFromCommand finds absolute paths in shell command strings.
func extractPathsFromCommand(cmd string, pathSet map[string]int) {
	// Split on common delimiters and check each token
	for _, token := range strings.FieldsFunc(cmd, func(r rune) bool {
		return r == ' ' || r == '"' || r == '\'' || r == '(' || r == ')' || r == ';' || r == '|' || r == '&'
	}) {
		token = strings.TrimSpace(token)
		if isAbsolutePath(token) {
			pathSet[normalizePath(token)]++
		}
	}
}

func isAbsolutePath(s string) bool {
	if len(s) < 5 {
		return false
	}
	return winPathRe.MatchString(s) || unixPathRe.MatchString(s)
}

func normalizePath(p string) string {
	// Unescape JSON backslashes
	return strings.ReplaceAll(p, `\\`, `\`)
}

// findRoots groups paths by their common directory roots.
func findRoots(pathSet map[string]int) []DetectedPath {
	rootCounts := make(map[string]int)
	for p, count := range pathSet {
		root := findMeaningfulRoot(p)
		if root != "" {
			rootCounts[root] += count
		}
	}

	// Deduplicate: if one root is prefix of another, keep only shorter
	var rootList []string
	for r := range rootCounts {
		rootList = append(rootList, r)
	}
	sort.Strings(rootList)

	var filtered []string
	for i, r := range rootList {
		isSubPath := false
		for j := 0; j < i; j++ {
			parent := rootList[j]
			sep := string(filepath.Separator)
			if strings.HasPrefix(r, parent+sep) || strings.HasPrefix(r, parent+"/") {
				rootCounts[parent] += rootCounts[r]
				isSubPath = true
				break
			}
		}
		if !isSubPath {
			filtered = append(filtered, r)
		}
	}

	var result []DetectedPath
	for _, r := range filtered {
		// Skip Claude's internal paths
		if strings.Contains(r, ".claude") {
			continue
		}
		if len(r) < 10 {
			continue
		}
		result = append(result, DetectedPath{
			Original: r,
			Count:    rootCounts[r],
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	return result
}

// findMeaningfulRoot returns a directory root at a meaningful depth.
// Takes up to 5 path components to group related files.
func findMeaningfulRoot(p string) string {
	normalized := filepath.ToSlash(p)
	parts := strings.Split(normalized, "/")

	if len(parts) < 2 {
		return ""
	}

	maxDepth := 5
	if len(parts) <= maxDepth {
		dir := filepath.Dir(p)
		if dir == p {
			return p
		}
		return dir
	}

	root := strings.Join(parts[:maxDepth], "/")
	return filepath.FromSlash(root)
}
