package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	rlmclients "github.com/O-guardiao/arkhe-go/rlm-go/clients"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/providers/openai_compat"
)

type rlmEmbeddedClient struct {
	rlmclients.BaseClient
	provider *openai_compat.Provider
}

func newRLMEmbeddedClient(backend string, backendKwargs map[string]any) (rlmclients.Client, error) {
	switch backend {
	case "openai", "vllm":
	default:
		return nil, fmt.Errorf("unsupported embedded rlm backend %q", backend)
	}

	modelName := rlmClientString(backendKwargs, "model_name")
	if modelName == "" {
		return nil, fmt.Errorf("embedded rlm client requires model_name")
	}
	baseURL := rlmClientString(backendKwargs, "base_url")
	if baseURL == "" {
		return nil, fmt.Errorf("embedded rlm client requires base_url")
	}
	timeout := rlmClientDurationSeconds(backendKwargs, "timeout")
	provider := openai_compat.NewProvider(
		rlmClientString(backendKwargs, "api_key"),
		baseURL,
		"",
		openai_compat.WithRequestTimeout(timeout),
		openai_compat.WithCustomHeaders(rlmClientStringMap(backendKwargs, "default_headers")),
	)

	return &rlmEmbeddedClient{
		BaseClient: rlmclients.NewBaseClient(modelName, timeout),
		provider:   provider,
	}, nil
}

func (c *rlmEmbeddedClient) Completion(ctx context.Context, prompt any, model string) (string, error) {
	messages, err := normalizeRLMEmbeddedMessages(prompt)
	if err != nil {
		return "", err
	}
	if model == "" {
		model = c.ModelName()
	}
	if model == "" {
		return "", fmt.Errorf("model name is required")
	}

	response, err := c.provider.Chat(ctx, messages, nil, model, nil)
	if err != nil {
		return "", err
	}
	inputTokens := 0
	outputTokens := 0
	if response.Usage != nil {
		inputTokens = response.Usage.PromptTokens
		outputTokens = response.Usage.CompletionTokens
	}
	c.TrackUsage(model, inputTokens, outputTokens, nil)
	return response.Content, nil
}

func normalizeRLMEmbeddedMessages(prompt any) ([]Message, error) {
	switch value := prompt.(type) {
	case string:
		return []Message{{Role: "user", Content: value}}, nil
	case map[string]any:
		message, err := normalizeRLMEmbeddedMessage(value)
		if err != nil {
			return nil, err
		}
		return []Message{message}, nil
	case []map[string]any:
		messages := make([]Message, 0, len(value))
		for _, item := range value {
			message, err := normalizeRLMEmbeddedMessage(item)
			if err != nil {
				return nil, err
			}
			messages = append(messages, message)
		}
		return messages, nil
	case []any:
		messages := make([]Message, 0, len(value))
		for _, item := range value {
			raw, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid message item type: %T", item)
			}
			message, err := normalizeRLMEmbeddedMessage(raw)
			if err != nil {
				return nil, err
			}
			messages = append(messages, message)
		}
		return messages, nil
	default:
		return nil, fmt.Errorf("invalid prompt type: %T", prompt)
	}
}

func normalizeRLMEmbeddedMessage(raw map[string]any) (Message, error) {
	message := Message{
		Role:    rlmClientStringDefault(raw, "role", "user"),
		Content: rlmContentString(raw["content"]),
	}
	if reasoning := rlmClientString(raw, "reasoning_content"); reasoning != "" {
		message.ReasoningContent = reasoning
	}
	if media := rlmStringSlice(raw["media"]); len(media) > 0 {
		message.Media = media
	}
	if toolCallID := rlmClientString(raw, "tool_call_id"); toolCallID != "" {
		message.ToolCallID = toolCallID
	}
	if rawToolCalls, ok := raw["tool_calls"]; ok {
		toolCalls, err := normalizeRLMEmbeddedToolCalls(rawToolCalls)
		if err != nil {
			return Message{}, err
		}
		message.ToolCalls = toolCalls
	}
	return message, nil
}

func normalizeRLMEmbeddedToolCalls(raw any) ([]ToolCall, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, nil
	}
	toolCalls := make([]ToolCall, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid tool call type: %T", item)
		}
		toolCall := ToolCall{
			ID:   rlmClientString(entry, "id"),
			Name: rlmClientString(entry, "name"),
		}
		if args, ok := entry["arguments"].(map[string]any); ok {
			toolCall.Arguments = args
		}
		if rawFunction, ok := entry["function"].(map[string]any); ok {
			toolCall.Function = &FunctionCall{
				Name:      rlmClientString(rawFunction, "name"),
				Arguments: rlmContentString(rawFunction["arguments"]),
			}
		}
		toolCalls = append(toolCalls, toolCall)
	}
	return toolCalls, nil
}

func rlmContentString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(raw)
	}
}

func rlmClientString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	return rlmContentString(values[key])
}

func rlmClientStringDefault(values map[string]any, key, fallback string) string {
	value := rlmClientString(values, key)
	if value == "" {
		return fallback
	}
	return value
}

func rlmClientDurationSeconds(values map[string]any, key string) time.Duration {
	if values == nil {
		return 0
	}
	switch typed := values[key].(type) {
	case int:
		return time.Duration(typed) * time.Second
	case int64:
		return time.Duration(typed) * time.Second
	case float64:
		return time.Duration(typed * float64(time.Second))
	default:
		return 0
	}
}

func rlmClientStringMap(values map[string]any, key string) map[string]string {
	if values == nil {
		return nil
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}
	switch typed := raw.(type) {
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, value := range typed {
			cloned[key] = value
		}
		return cloned
	case map[string]any:
		cloned := make(map[string]string, len(typed))
		for key, value := range typed {
			cloned[key] = fmt.Sprint(value)
		}
		return cloned
	default:
		return nil
	}
}

func rlmStringSlice(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			items = append(items, fmt.Sprint(item))
		}
		return items
	default:
		return nil
	}
}

