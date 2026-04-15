package clients

import "fmt"

func NewClient(backend string, backendKwargs map[string]any) (Client, error) {
	switch backend {
	case "mock":
		modelName := getString(backendKwargs, "model_name", "mock-model")
		responses := []string{}
		if rawResponses, ok := backendKwargs["responses"].([]string); ok {
			responses = append(responses, rawResponses...)
		} else if rawResponses, ok := backendKwargs["responses"].([]any); ok {
			for _, item := range rawResponses {
				responses = append(responses, fmt.Sprint(item))
			}
		}
		return NewMockClient(modelName, responses, nil), nil
	case "openai":
		return NewOpenAICompatibleClient(backendKwargs)
	case "vllm":
		return NewOpenAICompatibleClient(backendKwargs)
	case "openrouter":
		if backendKwargs == nil {
			backendKwargs = map[string]any{}
		}
		if _, ok := backendKwargs["base_url"]; !ok {
			backendKwargs["base_url"] = "https://openrouter.ai/api/v1"
		}
		return NewOpenAICompatibleClient(backendKwargs)
	case "vercel":
		if backendKwargs == nil {
			backendKwargs = map[string]any{}
		}
		if _, ok := backendKwargs["base_url"]; !ok {
			backendKwargs["base_url"] = "https://ai-gateway.vercel.sh/v1"
		}
		return NewOpenAICompatibleClient(backendKwargs)
	case "portkey":
		if backendKwargs == nil {
			backendKwargs = map[string]any{}
		}
		if _, ok := backendKwargs["base_url"]; !ok {
			backendKwargs["base_url"] = "https://api.portkey.ai/v1"
		}
		return NewOpenAICompatibleClient(backendKwargs)
	case "azure_openai":
		return NewAzureOpenAIClient(backendKwargs)
	case "anthropic":
		return NewAnthropicClient(backendKwargs)
	case "gemini":
		return NewGeminiClient(backendKwargs)
	default:
		return nil, fmt.Errorf("unknown backend: %s", backend)
	}
}
