// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/fileutil"
)

// MemoryStore manages persistent memory for the agent.
// - Long-term memory: memory/MEMORY.md
// - Daily notes: memory/YYYYMM/YYYYMMDD.md
type MemoryStore struct {
	workspace  string
	memoryDir  string
	memoryFile string
}

// NewMemoryStore creates a new MemoryStore with the given workspace path.
// It ensures the memory directory exists.
func NewMemoryStore(workspace string) *MemoryStore {
	memoryDir := filepath.Join(workspace, "memory")
	memoryFile := filepath.Join(memoryDir, "MEMORY.md")

	// Ensure memory directory exists
	os.MkdirAll(memoryDir, 0o755)

	return &MemoryStore{
		workspace:  workspace,
		memoryDir:  memoryDir,
		memoryFile: memoryFile,
	}
}

// getTodayFile returns the path to today's daily note file (memory/YYYYMM/YYYYMMDD.md).
func (ms *MemoryStore) getTodayFile() string {
	today := time.Now().Format("20060102") // YYYYMMDD
	monthDir := today[:6]                  // YYYYMM
	filePath := filepath.Join(ms.memoryDir, monthDir, today+".md")
	return filePath
}

// ReadLongTerm reads the long-term memory (MEMORY.md).
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadLongTerm() string {
	if data, err := os.ReadFile(ms.memoryFile); err == nil {
		return string(data)
	}
	return ""
}

// WriteLongTerm writes content to the long-term memory file (MEMORY.md).
func (ms *MemoryStore) WriteLongTerm(content string) error {
	// Use unified atomic write utility with explicit sync for flash storage reliability.
	// Using 0o600 (owner read/write only) for secure default permissions.
	return fileutil.WriteFileAtomic(ms.memoryFile, []byte(content), 0o600)
}

// ReadToday reads today's daily note.
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadToday() string {
	todayFile := ms.getTodayFile()
	if data, err := os.ReadFile(todayFile); err == nil {
		return string(data)
	}
	return ""
}

// AppendToday appends content to today's daily note.
// If the file doesn't exist, it creates a new file with a date header.
func (ms *MemoryStore) AppendToday(content string) error {
	todayFile := ms.getTodayFile()

	// Ensure month directory exists
	monthDir := filepath.Dir(todayFile)
	if err := os.MkdirAll(monthDir, 0o755); err != nil {
		return err
	}

	var existingContent string
	if data, err := os.ReadFile(todayFile); err == nil {
		existingContent = string(data)
	}

	var newContent string
	if existingContent == "" {
		// Add header for new day
		header := fmt.Sprintf("# %s\n\n", time.Now().Format("2006-01-02"))
		newContent = header + content
	} else {
		// Append to existing content
		newContent = existingContent + "\n" + content
	}

	// Use unified atomic write utility with explicit sync for flash storage reliability.
	return fileutil.WriteFileAtomic(todayFile, []byte(newContent), 0o600)
}

// GetRecentDailyNotes returns daily notes from the last N days.
// Contents are joined with "---" separator.
func (ms *MemoryStore) GetRecentDailyNotes(days int) string {
	var sb strings.Builder
	first := true

	for i := range days {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102") // YYYYMMDD
		monthDir := dateStr[:6]            // YYYYMM
		filePath := filepath.Join(ms.memoryDir, monthDir, dateStr+".md")

		if data, err := os.ReadFile(filePath); err == nil {
			if !first {
				sb.WriteString("\n\n---\n\n")
			}
			sb.Write(data)
			first = false
		}
	}

	return sb.String()
}

// GetMemoryContext returns formatted memory context for the agent prompt.
// Includes long-term memory and recent daily notes, with intelligent
// truncation to stay within a reasonable token budget (~500 tokens).
func (ms *MemoryStore) GetMemoryContext() string {
	longTerm := ms.ReadLongTerm()
	recentNotes := ms.GetRecentDailyNotes(3)

	if longTerm == "" && recentNotes == "" {
		return ""
	}

	var sb strings.Builder

	if longTerm != "" {
		sb.WriteString("## Long-term Memory\n\n")
		// Truncate long-term memory to ~1500 chars (~375 tokens) to leave
		// room for recent notes and avoid bloating the system prompt.
		truncated := truncateToLines(longTerm, 1500)
		sb.WriteString(truncated)
	}

	if recentNotes != "" {
		if longTerm != "" {
			sb.WriteString("\n\n---\n\n")
		}
		sb.WriteString("## Recent Notes\n\n")
		// Truncate recent notes to ~1000 chars (~250 tokens).
		truncated := truncateToLines(recentNotes, 1000)
		sb.WriteString(truncated)
	}

	return sb.String()
}

// ConsolidateDaily merges recent daily notes into MEMORY.md.
// This is a mechanical process (no LLM calls): deduplication, grouping,
// and formatting. Called automatically when daily notes exceed thresholds.
func (ms *MemoryStore) ConsolidateDaily() error {
	// Gather the last 7 days of notes.
	recentNotes := ms.GetRecentDailyNotes(7)
	if recentNotes == "" {
		return nil
	}

	// Parse entries from daily notes (lines starting with "- [").
	entries := extractNoteEntries(recentNotes)
	if len(entries) == 0 {
		return nil
	}

	// Deduplicate exact entries.
	entries = deduplicateEntries(entries)

	// Read existing long-term memory.
	existing := ms.ReadLongTerm()

	// Build consolidated memory.
	var sb strings.Builder
	sb.WriteString("# Long-term Memory\n\n")

	// Preserve any existing content that isn't auto-generated sections.
	existingCustom := extractCustomSections(existing)
	if existingCustom != "" {
		sb.WriteString(existingCustom)
		sb.WriteString("\n\n")
	}

	// Add auto-consolidated section.
	sb.WriteString("## Activity Log (auto-consolidated)\n\n")
	for _, entry := range entries {
		sb.WriteString(entry)
		sb.WriteString("\n")
	}

	return ms.WriteLongTerm(sb.String())
}

// truncateToLines truncates content to approximately maxChars by dropping
// trailing lines. Always keeps at least the first line.
func truncateToLines(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}

	lines := strings.Split(content, "\n")
	var sb strings.Builder
	for _, line := range lines {
		if sb.Len()+len(line)+1 > maxChars && sb.Len() > 0 {
			sb.WriteString("\n[... truncated]")
			break
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// extractNoteEntries parses lines starting with "- [" from daily note content.
func extractNoteEntries(content string) []string {
	lines := strings.Split(content, "\n")
	var entries []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [") {
			entries = append(entries, trimmed)
		}
	}
	return entries
}

// deduplicateEntries removes exact duplicate entries, preserving order.
func deduplicateEntries(entries []string) []string {
	seen := make(map[string]struct{}, len(entries))
	result := make([]string, 0, len(entries))
	for _, e := range entries {
		if _, ok := seen[e]; ok {
			continue
		}
		seen[e] = struct{}{}
		result = append(result, e)
	}
	return result
}

// extractCustomSections returns content from MEMORY.md that isn't
// auto-generated. Strips the "# Long-term Memory" header and the
// "## Activity Log (auto-consolidated)" section.
func extractCustomSections(content string) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	var sb strings.Builder
	inAutoSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip the main header.
		if trimmed == "# Long-term Memory" {
			continue
		}
		// Detect auto-consolidated section start.
		if strings.HasPrefix(trimmed, "## Activity Log") {
			inAutoSection = true
			continue
		}
		// A new ## heading ends the auto section.
		if inAutoSection && strings.HasPrefix(trimmed, "## ") {
			inAutoSection = false
		}
		if inAutoSection {
			continue
		}
		if sb.Len() > 0 || trimmed != "" {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	return strings.TrimSpace(sb.String())
}
