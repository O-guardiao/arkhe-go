package clients

import (
	"context"
	"fmt"
)

type OpenAICompatibleClient struct {
	BaseClient
	apiKey         string
	baseURL        string
	defaultHeaders map[string]string
	defaultQuery   map[string]string
}

func NewOpenAICompatibleClient(config map[string]any) (*OpenAICompatibleClient, error) {
	modelName := getString(config, "model_name", "")
	timeout := getFloat(config, "timeout", DefaultTimeout)
	baseURL := getString(config, "base_url", "https://api.openai.com/v1")
	apiKey := detectAPIKey(baseURL, getString(config, "api_key", ""))
	client := &OpenAICompatibleClient{
		BaseClient:     NewBaseClient(modelName, timeout),
		apiKey:         apiKey,
		baseURL:        baseURL,
		defaultHeaders: getMap(config, "default_headers"),
		defaultQuery:   getMap(config, "default_query"),
	}
	return client, nil
}

func (c *OpenAICompatibleClient) Completion(ctx context.Context, prompt any, model string) (string, error) {
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

	url := stringsWithQuery(c.baseURL+"/chat/completions", c.defaultQuery)
	headers := map[string]string{}
	for key, value := range c.defaultHeaders {
		headers[key] = value
	}
	if c.apiKey != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}

	body := map[string]any{
		"model":    model,
		"messages": messages,
	}
	if c.baseURL == "https://api.pinference.ai/api/v1/" {
		body["usage"] = map[string]any{"include": true}
	}

	payload, err := doJSONRequest(ctx, c.httpClient, "POST", url, headers, body)
	if err != nil {
		return "", err
	}
	content, err := extractOpenAIChoice(payload)
	if err != nil {
		return "", err
	}
	inputTokens, outputTokens, cost := extractOpenAIUsage(payload)
	c.TrackUsage(model, inputTokens, outputTokens, cost)
	return content, nil
}

func stringsWithQuery(baseURL string, query map[string]string) string {
	if len(query) == 0 {
		return baseURL
	}
	params := "?"
	first := true
	for key, value := range query {
		if !first {
			params += "&"
		}
		first = false
		params += key + "=" + value
	}
	return baseURL + params
}
