package utils

import (
	"fmt"
	"strings"
)

const (
	DefaultContextLimit   = 128_000
	CharsPerTokenEstimate = 4
)

var ModelContextLimits = map[string]int{
	"gpt-5-nano":          272_000,
	"gpt-5":               272_000,
	"gpt-4o-mini":         128_000,
	"gpt-4o-2024":         128_000,
	"gpt-4o":              128_000,
	"gpt-4-turbo-preview": 128_000,
	"gpt-4-turbo":         128_000,
	"gpt-4-32k":           32_768,
	"gpt-4":               8_192,
	"gpt-3.5-turbo-16k":   16_385,
	"gpt-3.5-turbo":       16_385,
	"o1-mini":             128_000,
	"o1-preview":          128_000,
	"o1":                  200_000,
	"claude-3-5-sonnet":   200_000,
	"claude-3-5-haiku":    200_000,
	"claude-3-opus":       200_000,
	"claude-3-sonnet":     200_000,
	"claude-3-haiku":      200_000,
	"claude-2.1":          200_000,
	"claude-2":            100_000,
	"gemini-2.5-flash":    1_000_000,
	"gemini-2.5-pro":      1_000_000,
	"gemini-2.0-flash":    1_000_000,
	"gemini-1.5-pro":      1_000_000,
	"gemini-1.5-flash":    1_000_000,
	"gemini-1.0-pro":      30_720,
	"qwen3-max":           256_000,
	"qwen3-72b":           128_000,
	"qwen3-32b":           128_000,
	"qwen3-8b":            32_768,
	"qwen3":               128_000,
	"kimi-k2.5":           262_000,
	"kimi-k2-0905":        256_000,
	"kimi-k2-thinking":    256_000,
	"kimi-k2":             128_000,
	"kimi":                128_000,
	"glm-4.6":             200_000,
	"glm-4-9b":            1_000_000,
	"glm-4":               128_000,
	"glm":                 128_000,
}

func GetContextLimit(modelName string) int {
	if modelName == "" || modelName == "unknown" {
		return DefaultContextLimit
	}
	if exact, ok := ModelContextLimits[modelName]; ok {
		return exact
	}
	bestLen := 0
	bestLimit := DefaultContextLimit
	for key, limit := range ModelContextLimits {
		if strings.Contains(modelName, key) && len(key) > bestLen {
			bestLen = len(key)
			bestLimit = limit
		}
	}
	return bestLimit
}

func CountTokens(messages []map[string]any, modelName string) int {
	if len(messages) == 0 {
		return 0
	}
	totalChars := 0
	for _, message := range messages {
		totalChars += len(fmt.Sprint(message["content"]))
	}
	return (totalChars + CharsPerTokenEstimate - 1) / CharsPerTokenEstimate
}
