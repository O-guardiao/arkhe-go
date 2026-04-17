package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/logger"
)

// memoryExtractorHook is a builtin EventObserver that watches TurnEnd events
// and extracts concise knowledge from significant turns into daily notes.
// Over time, these notes are consolidated into MEMORY.md (long-term memory).
type memoryExtractorHook struct {
	memory *MemoryStore
}

// newMemoryExtractorHook creates a memory extractor from the builtin hook config.
// The workspace path is extracted from config.Config (passed via BuiltinHookConfig).
func newMemoryExtractorHook(_ context.Context, workspace string) (*memoryExtractorHook, error) {
	if workspace == "" {
		return nil, fmt.Errorf("memory_extractor: workspace path is required")
	}
	return &memoryExtractorHook{
		memory: NewMemoryStore(workspace),
	}, nil
}

// OnEvent implements EventObserver. It fires on TurnEnd and extracts
// knowledge from significant turns into daily notes.
func (h *memoryExtractorHook) OnEvent(_ context.Context, evt Event) error {
	if evt.Kind != EventKindTurnEnd {
		return nil
	}

	payload, ok := evt.Payload.(TurnEndPayload)
	if !ok {
		return nil
	}

	// Only extract from completed turns that are significant.
	if payload.Status != TurnEndStatusCompleted {
		return nil
	}
	if !h.isSignificant(payload, evt) {
		return nil
	}

	// Build a concise note from the turn metadata.
	note := h.buildNote(payload, evt)
	if note == "" {
		return nil
	}

	if err := h.memory.AppendToday(note); err != nil {
		logger.WarnCF("memory_extractor", "Failed to append daily note", map[string]any{
			"error": err.Error(),
		})
		return err
	}

	logger.DebugCF("memory_extractor", "Extracted turn knowledge", map[string]any{
		"iterations":  payload.Iterations,
		"duration_ms": payload.Duration.Milliseconds(),
		"content_len": payload.FinalContentLen,
		"note_len":    len(note),
	})

	// Check if consolidation is needed.
	h.maybeConsolidate()

	return nil
}

// isSignificant determines if a turn produced enough meaningful work
// to warrant extraction. Pure heuristic — no LLM calls.
func (h *memoryExtractorHook) isSignificant(payload TurnEndPayload, evt Event) bool {
	// Multiple iterations suggest non-trivial reasoning.
	if payload.Iterations >= 3 {
		return true
	}

	// Long turns (>30s) typically involve tool usage or complex work.
	if payload.Duration > 30*time.Second {
		return true
	}

	// Substantial output suggests a meaningful response.
	if payload.FinalContentLen > 500 {
		return true
	}

	// Check meta for RLM reasoning traces.
	if evt.Meta.Source == "turn.end" && payload.Iterations >= 2 {
		return true
	}

	return false
}

// buildNote creates a concise daily note entry from turn metadata.
// This is a heuristic extraction — no LLM call needed.
func (h *memoryExtractorHook) buildNote(payload TurnEndPayload, evt Event) string {
	var sb strings.Builder

	timestamp := evt.Time.Format("15:04")
	fmt.Fprintf(&sb, "- [%s]", timestamp)

	// Describe what happened.
	if payload.Iterations >= 3 {
		fmt.Fprintf(&sb, " Multi-step reasoning (%d iterations, %.1fs)", payload.Iterations, payload.Duration.Seconds())
	} else {
		fmt.Fprintf(&sb, " Turn completed (%d iter, %.1fs)", payload.Iterations, payload.Duration.Seconds())
	}

	// Add agent context if available.
	if evt.Meta.AgentID != "" {
		fmt.Fprintf(&sb, " [agent:%s]", evt.Meta.AgentID)
	}

	// Add content length indicator.
	if payload.FinalContentLen > 1000 {
		fmt.Fprintf(&sb, " [long response: %d chars]", payload.FinalContentLen)
	}

	return sb.String()
}

// maybeConsolidate checks if daily notes have accumulated enough to
// warrant consolidation into MEMORY.md.
func (h *memoryExtractorHook) maybeConsolidate() {
	todayContent := h.memory.ReadToday()
	if todayContent == "" {
		return
	}

	// Count entries (lines starting with "- [").
	lines := strings.Split(todayContent, "\n")
	entryCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "- [") {
			entryCount++
		}
	}

	// Consolidate when >20 entries or >2000 chars accumulated today.
	if entryCount > 20 || len(todayContent) > 2000 {
		if err := h.memory.ConsolidateDaily(); err != nil {
			logger.WarnCF("memory_extractor", "Consolidation failed", map[string]any{
				"error": err.Error(),
			})
		}
	}
}
