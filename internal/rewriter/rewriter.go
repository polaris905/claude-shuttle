// Copyright (c) 2026 Cong Li
// Licensed under the MIT License. See LICENSE file in the project root for details.

package rewriter

import (
	"bufio"
	"os"
	"strings"
)

// RewritePaths reads a JSONL file line-by-line and replaces all occurrences
// of oldPath with newPath. Uses string replacement to preserve exact JSON format.
// Handles both forward-slash and backslash-escaped variants.
func RewritePaths(jsonlPath string, oldProjectPath string, newProjectPath string) error {
	if oldProjectPath == newProjectPath {
		return nil
	}

	tmpPath := jsonlPath + ".rewrite.tmp"

	in, err := os.Open(jsonlPath)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer out.Close()

	writer := bufio.NewWriter(out)

	// Build replacement pairs for different path representations:
	// 1. Raw path as-is (e.g., in non-escaped contexts)
	// 2. JSON-escaped backslashes: C:\\Users\\... (how it appears in JSON strings)
	// 3. Forward-slash variant: C:/Users/...
	type replacement struct {
		old string
		new string
	}
	var replacements []replacement

	// JSON-escaped backslash form (most common in JSONL)
	oldEscaped := strings.ReplaceAll(oldProjectPath, `\`, `\\`)
	newEscaped := strings.ReplaceAll(newProjectPath, `\`, `\\`)
	if oldEscaped != newEscaped {
		replacements = append(replacements, replacement{oldEscaped, newEscaped})
	}

	// Forward-slash form
	oldFwd := strings.ReplaceAll(oldProjectPath, `\`, `/`)
	newFwd := strings.ReplaceAll(newProjectPath, `\`, `/`)
	if oldFwd != newFwd {
		replacements = append(replacements, replacement{oldFwd, newFwd})
	}

	// Raw form (if different from above)
	if oldProjectPath != oldEscaped && oldProjectPath != oldFwd {
		replacements = append(replacements, replacement{oldProjectPath, newProjectPath})
	}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 10*1024*1024), 10*1024*1024) // 10MB buffer for large lines
	for scanner.Scan() {
		line := scanner.Text()
		for _, r := range replacements {
			line = strings.ReplaceAll(line, r.old, r.new)
		}
		writer.WriteString(line)
		writer.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	if err := writer.Flush(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	out.Close()
	in.Close()

	// Atomic replace
	return os.Rename(tmpPath, jsonlPath)
}
