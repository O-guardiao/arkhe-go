package recursion

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/providers"
)

// mockLLM is a minimal LLMCaller for compaction tests.
type mockLLM struct {
	reply    string
	err      error
	calls    int
	lastMsgs []providers.Message
}

func (m *mockLLM) Chat(
	_ context.Context,
	messages []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	m.calls++
	m.lastMsgs = messages
	if m.err != nil {
		return nil, m.err
	}
	return &providers.LLMResponse{Content: m.reply}, nil
}

func bigMsg(role string, n int) providers.Message {
	return providers.Message{Role: role, Content: strings.Repeat("x", n)}
}

func TestCompactMessages_BelowThresholdNoOp(t *testing.T) {
	llm := &mockLLM{reply: "summary"}
	msgs := []providers.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
	}
	_, changed, err := compactMessages(context.Background(), llm, "m", msgs, 0.8, 5, 1000)
	if err != nil || changed {
		t.Fatalf("expected no compaction, got changed=%v err=%v", changed, err)
	}
	if llm.calls != 0 {
		t.Fatalf("LLM should not be called below threshold, calls=%d", llm.calls)
	}
}

func TestCompactMessages_NilCallerNoOp(t *testing.T) {
	msgs := []providers.Message{bigMsg("user", 4000), bigMsg("assistant", 4000)}
	out, changed, err := compactMessages(context.Background(), nil, "m", msgs, 0.5, 1, 100)
	if err != nil || changed {
		t.Fatalf("nil caller must no-op, got changed=%v err=%v", changed, err)
	}
	if len(out) != len(msgs) {
		t.Fatalf("messages must be unchanged with nil caller")
	}
}

func TestCompactMessages_ZeroWindowNoOp(t *testing.T) {
	llm := &mockLLM{reply: "summary"}
	msgs := []providers.Message{bigMsg("user", 4000), bigMsg("assistant", 4000), bigMsg("user", 4000)}
	_, changed, err := compactMessages(context.Background(), llm, "m", msgs, 0.5, 1, 0)
	if err != nil || changed {
		t.Fatalf("zero context window must no-op, got changed=%v err=%v", changed, err)
	}
}

func TestCompactMessages_CompactsAndPreservesHeadAndTail(t *testing.T) {
	llm := &mockLLM{reply: "CONDENSED"}
	// contextWindow 100, threshold 0.5 => budget 50 tokens (~200 chars).
	msgs := []providers.Message{
		{Role: "system", Content: "system-prompt"},
		bigMsg("user", 400),      // middle
		bigMsg("assistant", 400), // middle
		bigMsg("user", 400),      // middle
		{Role: "assistant", Content: "tail-1"},
		{Role: "user", Content: "tail-2"},
	}
	out, changed, err := compactMessages(context.Background(), llm, "m", msgs, 0.5, 2, 100)
	if err != nil || !changed {
		t.Fatalf("expected compaction, got changed=%v err=%v", changed, err)
	}
	if llm.calls != 1 {
		t.Fatalf("expected 1 summarization call, got %d", llm.calls)
	}
	// system head preserved, then summary, then last 2 messages.
	if out[0].Role != "system" || out[0].Content != "system-prompt" {
		t.Fatalf("system head not preserved: %+v", out[0])
	}
	if !strings.Contains(out[1].Content, "CONDENSED") {
		t.Fatalf("summary message missing, got %+v", out[1])
	}
	if out[len(out)-1].Content != "tail-2" || out[len(out)-2].Content != "tail-1" {
		t.Fatalf("tail not preserved: %+v", out[len(out)-2:])
	}
	if len(out) != 4 { // system + summary + 2 tail
		t.Fatalf("unexpected length %d: %+v", len(out), out)
	}
}

func TestCompactMessages_NeverOrphansToolResult(t *testing.T) {
	llm := &mockLLM{reply: "S"}
	// keepLast=2 would place the cut right before a "tool" message; the cut must
	// snap forward so the preserved tail never starts with an orphan tool result.
	msgs := []providers.Message{
		bigMsg("user", 400),
		{Role: "assistant", Content: "call", ToolCalls: []providers.ToolCall{{Name: "bash", Arguments: map[string]any{"cmd": "ls"}}}},
		{Role: "tool", Content: strings.Repeat("y", 400), ToolCallID: "1"},
		{Role: "tool", Content: "second-result", ToolCallID: "2"},
	}
	out, changed, err := compactMessages(context.Background(), llm, "m", msgs, 0.5, 2, 100)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if changed {
		// If it compacted, the tail must not begin with a tool message.
		for i, m := range out {
			if m.Role == "user" && strings.HasPrefix(m.Content, "[Compacted") {
				if i+1 < len(out) && out[i+1].Role == "tool" {
					t.Fatalf("compaction left an orphan tool result right after summary: %+v", out)
				}
			}
		}
	}
	// Either way, no orphan tool result may appear without a preceding assistant
	// tool_call in the output.
	for i, m := range out {
		if m.Role == "tool" {
			if i == 0 || len(out[i-1].ToolCalls) == 0 && out[i-1].Role != "tool" {
				t.Fatalf("orphan tool result at index %d: %+v", i, out)
			}
		}
	}
}

func TestCompactMessages_LLMErrorIsFailSafe(t *testing.T) {
	llm := &mockLLM{err: errors.New("boom")}
	msgs := []providers.Message{
		{Role: "system", Content: "s"},
		bigMsg("user", 400),
		bigMsg("assistant", 400),
		bigMsg("user", 400),
		{Role: "user", Content: "last"},
	}
	out, changed, err := compactMessages(context.Background(), llm, "m", msgs, 0.5, 1, 100)
	if changed {
		t.Fatalf("must not change messages on LLM error")
	}
	if err == nil {
		t.Fatalf("expected the LLM error to propagate")
	}
	if len(out) != len(msgs) {
		t.Fatalf("messages must be returned unchanged on error")
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	msgs := []providers.Message{{Role: "user", Content: strings.Repeat("a", 40)}}
	// 40 content + 4 role = 44 chars / 4 = 11.
	if got := estimateMessagesTokens(msgs); got != 11 {
		t.Fatalf("estimate = %d, want 11", got)
	}
}
