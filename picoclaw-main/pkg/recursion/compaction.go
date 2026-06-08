package recursion

import (
	"context"
	"fmt"
	"strings"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/providers"
)

// compactionSystemPrompt instructs the LLM to condense an older slice of the
// conversation into a compact summary that preserves enough state for the agent
// to keep working without re-reading the full history.
const compactionSystemPrompt = "You are a context-compaction assistant for an autonomous agent. " +
	"Summarize the conversation excerpt below into a concise briefing that preserves all facts, " +
	"decisions, file paths, code, tool outputs, and unresolved tasks needed to continue the work. " +
	"Drop greetings and filler. Output only the summary."

// estimateMessagesTokens returns a rough token estimate for a message slice
// using the common ~4-characters-per-token heuristic. It intentionally errs on
// the cheap side; it only needs to be good enough to decide when the context is
// approaching the model window.
func estimateMessagesTokens(msgs []providers.Message) int {
	chars := 0
	for _, m := range msgs {
		chars += len(m.Role) + len(m.Content) + len(m.ReasoningContent)
		for _, tc := range m.ToolCalls {
			chars += len(tc.Name)
			if len(tc.Arguments) > 0 {
				chars += len(fmt.Sprintf("%v", tc.Arguments))
			}
		}
	}
	return chars / 4
}

// compactMessages condenses the middle of an oversized message history into a
// single summary message, preserving the leading system prompt(s) and the most
// recent keepLast messages. It returns the (possibly rewritten) slice and a
// flag indicating whether compaction actually happened.
//
// The function is fully fail-safe: when compaction is not configured, not
// needed, or the summarization call fails, it returns the original slice
// unchanged. It never splits a tool_call/tool_result pair — the boundary
// between the dropped middle and the preserved tail is snapped forward past any
// orphan "tool" role messages so providers never receive a dangling tool
// result.
func compactMessages(
	ctx context.Context,
	llm LLMCaller,
	model string,
	msgs []providers.Message,
	threshold float64,
	keepLast int,
	contextWindow int,
) ([]providers.Message, bool, error) {
	if llm == nil || contextWindow <= 0 || threshold <= 0 || keepLast <= 0 {
		return msgs, false, nil
	}

	budget := int(float64(contextWindow) * threshold)
	if budget <= 0 {
		return msgs, false, nil
	}
	if estimateMessagesTokens(msgs) <= budget {
		return msgs, false, nil
	}

	// Preserve leading system messages verbatim — they carry the agent's
	// instructions and must never be summarized away.
	headEnd := 0
	for headEnd < len(msgs) && msgs[headEnd].Role == "system" {
		headEnd++
	}

	// The tail is the most recent keepLast messages.
	cut := len(msgs) - keepLast
	if cut <= headEnd {
		return msgs, false, nil // not enough middle to be worth compacting
	}

	// Snap the cut forward so the preserved tail never starts with an orphan
	// tool result. Because every tool result immediately follows its assistant
	// tool_call, a non-"tool" message at cut guarantees the dropped middle ends
	// on a complete group.
	for cut < len(msgs) && msgs[cut].Role == "tool" {
		cut++
	}
	if cut >= len(msgs) || cut <= headEnd {
		return msgs, false, nil
	}

	middle := msgs[headEnd:cut]
	if len(middle) == 0 {
		return msgs, false, nil
	}

	summary, err := summarizeSegment(ctx, llm, model, middle)
	if err != nil {
		return msgs, false, err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return msgs, false, nil
	}

	out := make([]providers.Message, 0, headEnd+1+(len(msgs)-cut))
	out = append(out, msgs[:headEnd]...)
	out = append(out, providers.Message{
		Role:    "user",
		Content: "[Compacted summary of earlier turns]\n" + summary,
	})
	out = append(out, msgs[cut:]...)
	return out, true, nil
}

// summarizeSegment renders a message slice into a plain transcript and asks the
// LLM to condense it.
func summarizeSegment(
	ctx context.Context,
	llm LLMCaller,
	model string,
	segment []providers.Message,
) (string, error) {
	var sb strings.Builder
	for _, m := range segment {
		role := m.Role
		if role == "" {
			role = "message"
		}
		sb.WriteString(strings.ToUpper(role))
		sb.WriteString(": ")
		if m.Content != "" {
			sb.WriteString(m.Content)
		}
		for _, tc := range m.ToolCalls {
			sb.WriteString("\n[tool_call ")
			sb.WriteString(tc.Name)
			if len(tc.Arguments) > 0 {
				sb.WriteString(" ")
				sb.WriteString(fmt.Sprintf("%v", tc.Arguments))
			}
			sb.WriteString("]")
		}
		sb.WriteString("\n\n")
	}

	prompt := []providers.Message{
		{Role: "system", Content: compactionSystemPrompt},
		{Role: "user", Content: sb.String()},
	}

	resp, err := llm.Chat(ctx, prompt, nil, model, nil)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return resp.Content, nil
}
