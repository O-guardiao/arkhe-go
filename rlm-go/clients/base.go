package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/O-guardiao/arkhe-go/rlm-go/types"
)

const DefaultTimeout = 300 * time.Second

type Client interface {
	Completion(ctx context.Context, prompt any, model string) (string, error)
	GetUsageSummary() types.UsageSummary
	GetLastUsage() types.ModelUsageSummary
	ModelName() string
}

type BaseClient struct {
	modelName  string
	timeout    time.Duration
	httpClient *http.Client

	mu                sync.Mutex
	modelCallCounts   map[string]int
	modelInputTokens  map[string]int
	modelOutputTokens map[string]int
	modelCosts        map[string]float64
	lastUsage         types.ModelUsageSummary
}

func NewBaseClient(modelName string, timeout time.Duration) BaseClient {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return BaseClient{
		modelName: modelName,
		timeout:   timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		modelCallCounts:   map[string]int{},
		modelInputTokens:  map[string]int{},
		modelOutputTokens: map[string]int{},
		modelCosts:        map[string]float64{},
	}
}

func (b *BaseClient) ModelName() string {
	return b.modelName
}

func (b *BaseClient) TrackUsage(model string, inputTokens, outputTokens int, cost *float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.modelCallCounts[model]++
	b.modelInputTokens[model] += inputTokens
	b.modelOutputTokens[model] += outputTokens
	b.lastUsage = types.ModelUsageSummary{
		TotalCalls:        1,
		TotalInputTokens:  inputTokens,
		TotalOutputTokens: outputTokens,
		TotalCost:         cost,
	}
	if cost != nil {
		b.modelCosts[model] += *cost
	}
}

func (b *BaseClient) GetUsageSummary() types.UsageSummary {
	b.mu.Lock()
	defer b.mu.Unlock()

	models := map[string]types.ModelUsageSummary{}
	for model, calls := range b.modelCallCounts {
		var cost *float64
		if raw, ok := b.modelCosts[model]; ok && raw > 0 {
			value := raw
			cost = &value
		}
		models[model] = types.ModelUsageSummary{
			TotalCalls:        calls,
			TotalInputTokens:  b.modelInputTokens[model],
			TotalOutputTokens: b.modelOutputTokens[model],
			TotalCost:         cost,
		}
	}
	return types.UsageSummary{ModelUsageSummaries: models}
}

func (b *BaseClient) GetLastUsage() types.ModelUsageSummary {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastUsage
}

func getString(config map[string]any, key, fallback string) string {
	if config == nil {
		return fallback
	}
	if value, ok := config[key]; ok {
		if text, ok := value.(string); ok && text != "" {
			return text
		}
	}
	return fallback
}

func getFloat(config map[string]any, key string, fallback time.Duration) time.Duration {
	if config == nil {
		return fallback
	}
	switch value := config[key].(type) {
	case float64:
		return time.Duration(float64(time.Second) * value)
	case int:
		return time.Duration(value) * time.Second
	case int64:
		return time.Duration(value) * time.Second
	default:
		return fallback
	}
}

func getInt(config map[string]any, key string, fallback int) int {
	if config == nil {
		return fallback
	}
	switch value := config[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case int64:
		return int(value)
	default:
		return fallback
	}
}

func getMap(config map[string]any, key string) map[string]string {
	if config == nil {
		return nil
	}
	raw, ok := config[key]
	if !ok || raw == nil {
		return nil
	}
	switch value := raw.(type) {
	case map[string]string:
		return value
	case map[string]any:
		out := map[string]string{}
		for key, item := range value {
			out[key] = fmt.Sprint(item)
		}
		return out
	default:
		return nil
	}
}

func normalizeMessages(prompt any) ([]map[string]any, error) {
	switch value := prompt.(type) {
	case string:
		return []map[string]any{{"role": "user", "content": value}}, nil
	case []map[string]any:
		return value, nil
	case []any:
		out := make([]map[string]any, 0, len(value))
		for _, item := range value {
			msg, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid message item type: %T", item)
			}
			out = append(out, msg)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("invalid prompt type: %T", prompt)
	}
}

func doJSONRequest(
	ctx context.Context,
	client *http.Client,
	method string,
	url string,
	headers map[string]string,
	body any,
) (map[string]any, error) {
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	rawResponse, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{}
	if err := json.Unmarshal(rawResponse, &payload); err != nil {
		return nil, fmt.Errorf("decode %s: %w", response.Status, err)
	}
	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("%s: %s", response.Status, string(rawResponse))
	}
	return payload, nil
}

func extractOpenAIChoice(payload map[string]any) (string, error) {
	choices, ok := payload["choices"].([]any)
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("missing choices")
	}
	first, ok := choices[0].(map[string]any)
	if !ok {
		return "", fmt.Errorf("invalid choice payload")
	}
	message, ok := first["message"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("missing message")
	}
	return fmt.Sprint(message["content"]), nil
}

func extractOpenAIUsage(payload map[string]any) (int, int, *float64) {
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		return 0, 0, nil
	}
	inputTokens := intValue(usage["prompt_tokens"])
	outputTokens := intValue(usage["completion_tokens"])

	if cost := floatPointer(usage["cost"]); cost != nil {
		return inputTokens, outputTokens, cost
	}
	if modelExtra, ok := usage["model_extra"].(map[string]any); ok {
		if cost := floatPointer(modelExtra["cost"]); cost != nil {
			return inputTokens, outputTokens, cost
		}
		if costDetails, ok := modelExtra["cost_details"].(map[string]any); ok {
			if cost := floatPointer(costDetails["upstream_inference_cost"]); cost != nil {
				return inputTokens, outputTokens, cost
			}
		}
	}
	return inputTokens, outputTokens, nil
}

func intValue(value any) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	default:
		return 0
	}
}

func floatPointer(value any) *float64 {
	switch typed := value.(type) {
	case float64:
		out := typed
		return &out
	case int:
		out := float64(typed)
		return &out
	case string:
		if typed == "" {
			return nil
		}
		var out float64
		if _, err := fmt.Sscanf(typed, "%f", &out); err == nil {
			return &out
		}
	}
	return nil
}

func envOrFallback(primary string, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

func detectAPIKey(baseURL string, explicit string) string {
	if explicit != "" {
		return explicit
	}
	switch strings.TrimSuffix(baseURL, "/") {
	case "", "https://api.openai.com/v1":
		return os.Getenv("OPENAI_API_KEY")
	case "https://openrouter.ai/api/v1":
		return os.Getenv("OPENROUTER_API_KEY")
	case "https://ai-gateway.vercel.sh/v1":
		return os.Getenv("AI_GATEWAY_API_KEY")
	case "https://api.portkey.ai/v1":
		return os.Getenv("PORTKEY_API_KEY")
	default:
		return os.Getenv("OPENAI_API_KEY")
	}
}
