package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
)

func TestMemoryExtractor_IsSignificant(t *testing.T) {
	h := &memoryExtractorHook{memory: NewMemoryStore(t.TempDir())}

	tests := []struct {
		name    string
		payload TurnEndPayload
		want    bool
	}{
		{
			name:    "many iterations",
			payload: TurnEndPayload{Status: TurnEndStatusCompleted, Iterations: 5, Duration: 10 * time.Second},
			want:    true,
		},
		{
			name:    "long duration",
			payload: TurnEndPayload{Status: TurnEndStatusCompleted, Iterations: 1, Duration: 45 * time.Second},
			want:    true,
		},
		{
			name:    "large content",
			payload: TurnEndPayload{Status: TurnEndStatusCompleted, Iterations: 1, Duration: 5 * time.Second, FinalContentLen: 800},
			want:    true,
		},
		{
			name:    "trivial turn",
			payload: TurnEndPayload{Status: TurnEndStatusCompleted, Iterations: 1, Duration: 2 * time.Second, FinalContentLen: 50},
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			evt := Event{Kind: EventKindTurnEnd, Time: time.Now(), Meta: EventMeta{Source: "turn.end"}}
			if got := h.isSignificant(tc.payload, evt); got != tc.want {
				t.Fatalf("isSignificant(%+v) = %v, want %v", tc.payload, got, tc.want)
			}
		})
	}
}

func TestMemoryExtractor_OnEvent_AppendsDailyNote(t *testing.T) {
	workspace := t.TempDir()
	h := &memoryExtractorHook{memory: NewMemoryStore(workspace)}

	// Significant turn: 5 iterations.
	evt := Event{
		Kind: EventKindTurnEnd,
		Time: time.Now(),
		Meta: EventMeta{AgentID: "test-agent", Source: "turn.end"},
		Payload: TurnEndPayload{
			Status:          TurnEndStatusCompleted,
			Iterations:      5,
			Duration:        15 * time.Second,
			FinalContentLen: 200,
		},
	}

	if err := h.OnEvent(context.Background(), evt); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}

	// Read today's note and verify it was written.
	content := h.memory.ReadToday()
	if content == "" {
		t.Fatal("expected daily note to be written")
	}
	if !strings.Contains(content, "Multi-step reasoning") {
		t.Fatalf("expected multi-step note, got: %q", content)
	}
	if !strings.Contains(content, "agent:test-agent") {
		t.Fatalf("expected agent ID in note, got: %q", content)
	}
}

func TestMemoryExtractor_SkipsErrorTurns(t *testing.T) {
	workspace := t.TempDir()
	h := &memoryExtractorHook{memory: NewMemoryStore(workspace)}

	evt := Event{
		Kind: EventKindTurnEnd,
		Time: time.Now(),
		Payload: TurnEndPayload{
			Status:     TurnEndStatusError,
			Iterations: 10,
			Duration:   60 * time.Second,
		},
	}

	if err := h.OnEvent(context.Background(), evt); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}

	if content := h.memory.ReadToday(); content != "" {
		t.Fatalf("expected no note for error turn, got: %q", content)
	}
}

func TestMemoryExtractor_SkipsNonTurnEndEvents(t *testing.T) {
	workspace := t.TempDir()
	h := &memoryExtractorHook{memory: NewMemoryStore(workspace)}

	evt := Event{
		Kind: EventKindTurnStart,
		Time: time.Now(),
		Payload: TurnStartPayload{
			UserMessage: "hello",
		},
	}

	if err := h.OnEvent(context.Background(), evt); err != nil {
		t.Fatalf("OnEvent: %v", err)
	}

	if content := h.memory.ReadToday(); content != "" {
		t.Fatalf("expected no note for non-turn-end event, got: %q", content)
	}
}

func TestConsolidateDaily(t *testing.T) {
	workspace := t.TempDir()
	ms := NewMemoryStore(workspace)

	// Add multiple daily note entries.
	for i := 0; i < 5; i++ {
		if err := ms.AppendToday("- [10:0" + string(rune('0'+i)) + "] Test entry " + string(rune('A'+i))); err != nil {
			t.Fatalf("AppendToday: %v", err)
		}
	}

	// Add a duplicate.
	if err := ms.AppendToday("- [10:00] Test entry A"); err != nil {
		t.Fatalf("AppendToday duplicate: %v", err)
	}

	// Consolidate.
	if err := ms.ConsolidateDaily(); err != nil {
		t.Fatalf("ConsolidateDaily: %v", err)
	}

	// Read long-term memory.
	longTerm := ms.ReadLongTerm()
	if longTerm == "" {
		t.Fatal("expected long-term memory to be written")
	}
	if !strings.Contains(longTerm, "Activity Log") {
		t.Fatalf("expected Activity Log section, got: %q", longTerm)
	}
	// Count entries — should be 5 (deduplicated from 6).
	count := strings.Count(longTerm, "- [10:")
	if count != 5 {
		t.Fatalf("expected 5 deduplicated entries, got %d in: %q", count, longTerm)
	}
}

func TestConsolidateDaily_PreservesCustomSections(t *testing.T) {
	workspace := t.TempDir()
	ms := NewMemoryStore(workspace)

	// Write existing long-term memory with custom sections.
	existing := `# Long-term Memory

## User Preferences

User prefers concise responses.

## Activity Log (auto-consolidated)

- [09:00] Old entry
`
	if err := ms.WriteLongTerm(existing); err != nil {
		t.Fatalf("WriteLongTerm: %v", err)
	}

	// Add new daily notes.
	if err := ms.AppendToday("- [14:00] New entry today"); err != nil {
		t.Fatalf("AppendToday: %v", err)
	}

	// Consolidate.
	if err := ms.ConsolidateDaily(); err != nil {
		t.Fatalf("ConsolidateDaily: %v", err)
	}

	// Verify custom section preserved.
	longTerm := ms.ReadLongTerm()
	if !strings.Contains(longTerm, "User Preferences") {
		t.Fatalf("expected User Preferences section preserved, got: %q", longTerm)
	}
	if !strings.Contains(longTerm, "concise responses") {
		t.Fatalf("expected custom content preserved, got: %q", longTerm)
	}
	// Verify new entry present.
	if !strings.Contains(longTerm, "New entry today") {
		t.Fatalf("expected new entry, got: %q", longTerm)
	}
}

func TestTruncateToLines(t *testing.T) {
	short := "line 1\nline 2"
	if got := truncateToLines(short, 100); got != short {
		t.Fatalf("expected no truncation, got: %q", got)
	}

	long := "line 1\nline 2 is a bit longer\nline 3 also long"
	got := truncateToLines(long, 20)
	if !strings.Contains(got, "truncated") {
		t.Fatalf("expected truncation marker, got: %q", got)
	}
	if len(got) > 50 { // should be reasonably short
		t.Fatalf("truncated content too long: %d chars", len(got))
	}
}

func TestExtractNoteEntries(t *testing.T) {
	content := "# 2026-04-16\n\n- [10:00] Entry one\nsome other line\n- [10:05] Entry two\n"
	entries := extractNoteEntries(content)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0] != "- [10:00] Entry one" {
		t.Fatalf("unexpected entry: %q", entries[0])
	}
}

func TestDeduplicateEntries(t *testing.T) {
	entries := []string{"- [10:00] A", "- [10:01] B", "- [10:00] A", "- [10:02] C"}
	deduped := deduplicateEntries(entries)
	if len(deduped) != 3 {
		t.Fatalf("expected 3 deduped entries, got %d", len(deduped))
	}
}

func TestMemoryExtractorRegistration(t *testing.T) {
	// Verify memory_extractor is registered as a workspace hook.
	factory, ok := lookupWorkspaceHook("memory_extractor")
	if !ok {
		t.Fatal("memory_extractor not registered as workspace hook")
	}
	if factory == nil {
		t.Fatal("memory_extractor factory is nil")
	}

	// Create the hook from factory.
	workspace := t.TempDir()
	hook, err := factory(context.Background(), dummyBuiltinHookConfig(), workspace)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}

	// Verify it implements EventObserver.
	if _, ok := hook.(EventObserver); !ok {
		t.Fatal("memory_extractor hook does not implement EventObserver")
	}
}

func TestGetMemoryContextTruncation(t *testing.T) {
	workspace := t.TempDir()
	ms := NewMemoryStore(workspace)

	// Write a very large long-term memory.
	longContent := strings.Repeat("This is a long line of memory content.\n", 100)
	if err := ms.WriteLongTerm(longContent); err != nil {
		t.Fatalf("WriteLongTerm: %v", err)
	}

	ctx := ms.GetMemoryContext()
	if ctx == "" {
		t.Fatal("expected non-empty memory context")
	}

	// Should be truncated (original is ~3900 chars, limit is ~1500+overhead).
	if !strings.Contains(ctx, "truncated") {
		t.Fatalf("expected truncation marker in context, got %d chars", len(ctx))
	}
}

func dummyBuiltinHookConfig() config.BuiltinHookConfig {
	return config.BuiltinHookConfig{Enabled: true}
}

// Ensure the import resolves.
func TestMemoryExtractorHookWorkspaceDir(t *testing.T) {
	workspace := t.TempDir()
	h, err := newMemoryExtractorHook(context.Background(), workspace)
	if err != nil {
		t.Fatalf("newMemoryExtractorHook: %v", err)
	}

	// Verify memory dir was created.
	memDir := filepath.Join(workspace, "memory")
	if _, err := os.Stat(memDir); os.IsNotExist(err) {
		t.Fatal("expected memory directory to be created")
	}

	// Verify the hook has a valid memory store.
	if h.memory == nil {
		t.Fatal("expected non-nil memory store")
	}
}
