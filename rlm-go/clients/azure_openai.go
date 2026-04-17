package clients

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type AzureOpenAIClient struct {
	BaseClient
	apiKey          string
	azureEndpoint   string
	apiVersion      string
	azureDeployment string
}

func NewAzureOpenAIClient(config map[string]any) (*AzureOpenAIClient, error) {
	modelName := getString(config, "model_name", "")
	timeout := getFloat(config, "timeout", DefaultTimeout)
	client := &AzureOpenAIClient{
		BaseClient:      NewBaseClient(modelName, timeout),
		apiKey:          getString(config, "api_key", os.Getenv("AZURE_OPENAI_API_KEY")),
		azureEndpoint:   strings.TrimSuffix(getString(config, "azure_endpoint", os.Getenv("AZURE_OPENAI_ENDPOINT")), "/"),
		apiVersion:      getString(config, "api_version", os.Getenv("AZURE_OPENAI_API_VERSION")),
		azureDeployment: getString(config, "azure_deployment", os.Getenv("AZURE_OPENAI_DEPLOYMENT")),
	}
	if client.apiVersion == "" {
		client.apiVersion = "2024-02-01"
	}
	if client.azureEndpoint == "" {
		return nil, fmt.Errorf("azure endpoint is required")
	}
	if client.azureDeployment == "" {
		client.azureDeployment = modelName
	}
	return client, nil
}

func (c *AzureOpenAIClient) Completion(ctx context.Context, prompt any, model string) (string, error) {
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

	deployment := c.azureDeployment
	if deployment == "" {
		deployment = model
	}

	url := fmt.Sprintf(
		"%s/openai/deployments/%s/chat/completions?api-version=%s",
		c.azureEndpoint,
		deployment,
		c.apiVersion,
	)
	headers := map[string]string{
		"api-key": c.apiKey,
	}
	body := map[string]any{
		"model":    model,
		"messages": messages,
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
