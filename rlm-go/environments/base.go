package environments

import (
	"fmt"
	"sort"
	"strings"

	"github.com/O-guardiao/arkhe-go/rlm-go/types"
)

var ReservedToolNames = map[string]struct{}{
	"llm_query":         {},
	"llm_query_batched": {},
	"rlm_query":         {},
	"rlm_query_batched": {},
	"FINAL_VAR":         {},
	"SHOW_VARS":         {},
	"context":           {},
	"history":           {},
	"print":             {},
}

type ToolInfo struct {
	Name        string
	Value       any
	Description string
}

func (t ToolInfo) IsCallable() bool {
	switch t.Value.(type) {
	case func(...any), func(any) string, func(string) string, func(string, string) string:
		return true
	default:
		return fmt.Sprintf("%T", t.Value) != "<nil>" && strings.HasPrefix(fmt.Sprintf("%T", t.Value), "func(")
	}
}

func ParseToolEntry(name string, entry any) ToolInfo {
	if wrapped, ok := entry.(map[string]any); ok {
		if tool, ok := wrapped["tool"]; ok {
			description, _ := wrapped["description"].(string)
			return ToolInfo{Name: name, Value: tool, Description: description}
		}
	}
	return ToolInfo{Name: name, Value: entry}
}

func ParseCustomTools(customTools map[string]any) []ToolInfo {
	if len(customTools) == 0 {
		return nil
	}
	out := make([]ToolInfo, 0, len(customTools))
	for name, entry := range customTools {
		out = append(out, ParseToolEntry(name, entry))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func ExtractToolValue(entry any) any {
	if wrapped, ok := entry.(map[string]any); ok {
		if tool, ok := wrapped["tool"]; ok {
			return tool
		}
	}
	return entry
}

func FormatToolsForPrompt(customTools map[string]any) string {
	toolInfos := ParseCustomTools(customTools)
	if len(toolInfos) == 0 {
		return ""
	}
	lines := make([]string, 0, len(toolInfos))
	for _, tool := range toolInfos {
		if tool.Description != "" {
			lines = append(lines, fmt.Sprintf("- `%s`: %s", tool.Name, tool.Description))
			continue
		}
		lines = append(lines, fmt.Sprintf("- `%s`: custom %T", tool.Name, ExtractToolValue(tool.Value)))
	}
	return strings.Join(lines, "\n")
}

func ValidateCustomTools(customTools map[string]any) error {
	if len(customTools) == 0 {
		return nil
	}
	conflicts := []string{}
	for name := range customTools {
		if _, ok := ReservedToolNames[name]; ok {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	return fmt.Errorf("custom tools cannot override reserved REPL symbols: %v", conflicts)
}

type Environment interface {
	Setup() error
	LoadContext(contextPayload any) error
	ExecuteCode(code string) types.REPLResult
	Cleanup() error
}

type PersistentEnvironment interface {
	Environment
	UpdateHandlerAddress(address string)
	AddContext(contextPayload any, contextIndex *int) (int, error)
	GetContextCount() int
	AddHistory(messageHistory []map[string]any, historyIndex *int) (int, error)
	GetHistoryCount() int
}

type SubcallFn func(prompt string, model string) (types.RLMChatCompletion, error)

type Config struct {
	LMHandlerAddress      string
	ContextPayload        any
	SetupCode             string
	Persistent            bool
	Depth                 int
	SubcallFn             SubcallFn
	CustomTools           map[string]any
	CustomSubTools        map[string]any
	Compaction            bool
	MaxConcurrentSubcalls int
	WorkingDir            string
	WriteContextFiles     bool
}

func NewEnvironment(kind string, config Config) (Environment, error) {
	switch kind {
	case "", "local":
		return NewLocalREPL(config)
	default:
		return nil, fmt.Errorf("environment %q is not implemented in the Go clean-room runtime", kind)
	}
}
