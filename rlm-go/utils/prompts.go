package utils

import (
	"fmt"
	"strings"

	"github.com/O-guardiao/arkhe-go/rlm-go/environments"
	"github.com/O-guardiao/arkhe-go/rlm-go/types"
)

const RLMSystemPrompt = `You are solving a task using a persistent Go REPL environment.

The REPL exposes:
1. A ` + "`context`" + ` variable containing the primary input.
2. ` + "`llm_query(prompt, model)`" + ` for one-shot LM calls.
3. ` + "`llm_query_batched(prompts, model)`" + ` for concurrent one-shot LM calls.
4. ` + "`rlm_query(prompt, model)`" + ` for recursive child RLM calls.
5. ` + "`rlm_query_batched(prompts, model)`" + ` for concurrent recursive child RLM calls.
6. ` + "`SHOW_VARS()`" + ` to inspect persistent variables in the Go REPL.
7. ` + "`FINAL_VAR(nameOrValue)`" + ` to mark a final value created in REPL code.
{custom_tools_section}

The raw context is not fully in this chat history. It lives in the REPL as ` + "`context`" + ` and possibly ` + "`context_N`" + ` variables. Use programmatic exploration, chunking, summarization, and recursive calls to solve the task. When you execute code, wrap it in ` + "```repl" + ` blocks containing Go code.

When you are done, answer outside code using either:
- FINAL(your final answer)
- FINAL_VAR(variable_name)

Do real work immediately.`

const userPrompt = `Think step by step using the Go REPL environment and continue executing the task. Your next action:`
const userPromptWithRoot = `Think step by step using the Go REPL environment to answer the original prompt: "%s". Your next action:`

func BuildRLMSystemPrompt(
	systemPrompt string,
	queryMetadata types.QueryMetadata,
	customTools map[string]any,
) []map[string]any {
	contextLengths := fmt.Sprint(queryMetadata.ContextLengths)
	if len(queryMetadata.ContextLengths) > 100 {
		rest := len(queryMetadata.ContextLengths) - 100
		contextLengths = fmt.Sprintf("%v... [%d others]", queryMetadata.ContextLengths[:100], rest)
	}

	customToolsSection := ""
	if formatted := environments.FormatToolsForPrompt(customTools); formatted != "" {
		customToolsSection = "\n8. Custom tools and data available in the REPL:\n" + formatted
	}

	finalSystemPrompt := strings.ReplaceAll(systemPrompt, "{custom_tools_section}", customToolsSection)
	metadataPrompt := fmt.Sprintf(
		"Your context is a %s with %d total characters, broken into chunks with lengths: %s.",
		queryMetadata.ContextType,
		queryMetadata.ContextTotalLength,
		contextLengths,
	)

	return []map[string]any{
		{"role": "system", "content": finalSystemPrompt},
		{"role": "user", "content": metadataPrompt},
	}
}

func BuildUserPrompt(rootPrompt string, iteration, contextCount, historyCount int) map[string]any {
	var content string
	if iteration == 0 {
		safeguard := "You have not interacted with the REPL yet. Inspect the context and avoid jumping to a final answer.\n\n"
		if rootPrompt != "" {
			content = safeguard + fmt.Sprintf(userPromptWithRoot, rootPrompt)
		} else {
			content = safeguard + userPrompt
		}
	} else {
		prefix := "The prior messages describe your previous REPL interactions. "
		if rootPrompt != "" {
			content = prefix + fmt.Sprintf(userPromptWithRoot, rootPrompt)
		} else {
			content = prefix + userPrompt
		}
	}

	if contextCount > 1 {
		content += fmt.Sprintf("\n\nNote: you have %d contexts available (context_0 through context_%d).", contextCount, contextCount-1)
	}
	if historyCount > 0 {
		if historyCount == 1 {
			content += "\n\nNote: you have 1 prior conversation history in the `history` variable."
		} else {
			content += fmt.Sprintf("\n\nNote: you have %d prior histories available (history_0 through history_%d).", historyCount, historyCount-1)
		}
	}

	return map[string]any{"role": "user", "content": content}
}
