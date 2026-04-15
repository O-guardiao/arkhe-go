package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/alexzhang13/rlm-go/types"
)

var (
	replBlockPattern  = regexp.MustCompile("(?s)```repl\\s*\\n(.*?)\\n```")
	finalVarPattern   = regexp.MustCompile("(?ms)^\\s*FINAL_VAR\\((.*?)\\)")
	finalPlainPattern = regexp.MustCompile("(?ms)^\\s*FINAL\\((.*)\\)\\s*$")
)

type FinalAnswerResolver interface {
	ResolveFinalValue(name string) (string, bool)
}

func FindCodeBlocks(text string) []string {
	matches := replBlockPattern.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			out = append(out, strings.TrimSpace(match[1]))
		}
	}
	return out
}

func FindFinalAnswer(text string, resolver FinalAnswerResolver) (string, bool) {
	if match := finalVarPattern.FindStringSubmatch(text); len(match) > 1 {
		name := strings.TrimSpace(strings.Trim(match[1], `"'`))
		if resolver == nil {
			return "", false
		}
		return resolver.ResolveFinalValue(name)
	}
	if match := finalPlainPattern.FindStringSubmatch(text); len(match) > 1 {
		return strings.TrimSpace(match[1]), true
	}
	return "", false
}

func FormatIteration(iteration types.RLMIteration, maxCharacterLength int) []map[string]any {
	if maxCharacterLength <= 0 {
		maxCharacterLength = 20_000
	}
	messages := []map[string]any{
		{"role": "assistant", "content": iteration.Response},
	}
	for _, codeBlock := range iteration.CodeBlocks {
		formatted := FormatExecutionResult(codeBlock.Result)
		if len(formatted) > maxCharacterLength {
			formatted = formatted[:maxCharacterLength] + fmt.Sprintf("... + [%d chars...]", len(formatted)-maxCharacterLength)
		}
		messages = append(messages, map[string]any{
			"role": "user",
			"content": fmt.Sprintf(
				"Code executed:\n```go\n%s\n```\n\nREPL output:\n%s",
				codeBlock.Code,
				formatted,
			),
		})
	}
	return messages
}

func FormatExecutionResult(result types.REPLResult) string {
	parts := []string{}
	if result.Stdout != "" {
		parts = append(parts, "\n"+result.Stdout)
	}
	if result.Stderr != "" {
		parts = append(parts, "\n"+result.Stderr)
	}
	importantVars := []string{}
	for key, value := range result.Locals {
		if strings.HasPrefix(key, "_") || key == "__builtins__" || key == "__name__" || key == "__doc__" {
			continue
		}
		rv := reflect.ValueOf(value)
		if !rv.IsValid() {
			continue
		}
		switch rv.Kind() {
		case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64, reflect.Slice, reflect.Array, reflect.Map:
			importantVars = append(importantVars, key)
		}
	}
	if len(importantVars) > 0 {
		parts = append(parts, fmt.Sprintf("REPL variables: %v\n", importantVars))
	}
	if len(parts) == 0 {
		return "No output"
	}
	return strings.Join(parts, "\n\n")
}

func ConvertContextForREPL(context any) (any, *string) {
	switch value := context.(type) {
	case string:
		return nil, &value
	case map[string]any, []string, []map[string]any, []any:
		return value, nil
	default:
		raw, err := json.Marshal(value)
		if err == nil {
			var decoded any
			if json.Unmarshal(raw, &decoded) == nil {
				return decoded, nil
			}
		}
		return value, nil
	}
}
