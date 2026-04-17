package clients

import (
	"context"
	"fmt"
	"os"
)

type GeminiClient struct {
	BaseClient
	apiKey string
}

func NewGeminiClient(config map[string]any) (*GeminiClient, error) {
	modelName := getString(config, "model_name", "gemini-2.5-flash")
	timeout := getFloat(config, "timeout", DefaultTimeout)
	apiKey := getString(config, "api_key", os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("gemini api key is required")
	}
	return &GeminiClient{
		BaseClient: NewBaseClient(modelName, timeout),
		apiKey:     apiKey,
	}, nil
}

func (c *GeminiClient) Completion(ctx context.Context, prompt any, model string) (string, error) {
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

	contents := []map[string]any{}
	systemInstruction := ""
	for _, message := range messages {
		role := fmt.Sprint(message["role"])
		content := fmt.Sprint(message["content"])
		switch role {
		case "system":
			systemInstruction = content
		case "assistant":
			contents = append(contents, map[string]any{
				"role":  "model",
				"parts": []map[string]any{{"text": content}},
			})
		default:
			contents = append(contents, map[string]any{
				"role":  "user",
				"parts": []map[string]any{{"text": content}},
			})
		}
	}

	body := map[string]any{
		"contents": contents,
	}
	if systemInstruction != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": systemInstruction}},
		}
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent",
		model,
	)
	headers := map[string]string{
		"x-goog-api-key": c.apiKey,
	}
	payload, err := doJSONRequest(ctx, c.httpClient, "POST", url, headers, body)
	if err != nil {
		return "", err
	}

	candidates, ok := payload["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		return "", fmt.Errorf("missing gemini candidates")
	}
	first, ok := candidates[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid gemini candidate")
	}
	content, ok := first["content"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("missing gemini content")
	}
	parts, ok := content["parts"].([]any)
	if !ok || len(parts) == 0 {
		return "", fmt.Errorf("missing gemini parts")
	}
	part, ok := parts[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid gemini part")
	}
	text := fmt.Sprint(part["text"])

	usage, _ := payload["usageMetadata"].(map[string]any)
	inputTokens := intValue(usage["promptTokenCount"])
	outputTokens := intValue(usage["candidatesTokenCount"])
	c.TrackUsage(model, inputTokens, outputTokens, nil)
	return text, nil
}
