package clients

import (
	"context"
	"fmt"
	"os"
)

type AnthropicClient struct {
	BaseClient
	apiKey    string
	maxTokens int
}

func NewAnthropicClient(config map[string]any) (*AnthropicClient, error) {
	modelName := getString(config, "model_name", "")
	timeout := getFloat(config, "timeout", DefaultTimeout)
	apiKey := getString(config, "api_key", os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic api key is required")
	}
	maxTokens := getInt(config, "max_tokens", 32768)
	return &AnthropicClient{
		BaseClient: NewBaseClient(modelName, timeout),
		apiKey:     apiKey,
		maxTokens:  maxTokens,
	}, nil
}

func (c *AnthropicClient) Completion(ctx context.Context, prompt any, model string) (string, error) {
	messages, err := normalizeMessages(prompt)
	if err != nil {
		return "", err
	}
	if model == "" {
		model = c.ModelName()
	}
	if model == "" {
		return "", fmt.Errorf("model name is required")
	}

	apiMessages := []map[string]any{}
	system := ""
	for _, message := range messages {
		if fmt.Sprint(message["role"]) == "system" {
			system = fmt.Sprint(message["content"])
			continue
		}
		apiMessages = append(apiMessages, map[string]any{
			"role":    message["role"],
			"content": message["content"],
		})
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": c.maxTokens,
		"messages":   apiMessages,
	}
	if system != "" {
		body["system"] = system
	}

	headers := map[string]string{
		"x-api-key":         c.apiKey,
		"anthropic-version": "2023-06-01",
	}
	payload, err := doJSONRequest(ctx, c.httpClient, "POST", "https://api.anthropic.com/v1/messages", headers, body)
	if err != nil {
		return "", err
	}

	contentArray, ok := payload["content"].([]any)
	if !ok || len(contentArray) == 0 {
		return "", fmt.Errorf("missing anthropic content")
	}
	first, ok := contentArray[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid anthropic content")
	}
	content := fmt.Sprint(first["text"])

	usage, _ := payload["usage"].(map[string]any)
	inputTokens := intValue(usage["input_tokens"])
	outputTokens := intValue(usage["output_tokens"])
	c.TrackUsage(model, inputTokens, outputTokens, nil)
	return content, nil
}
