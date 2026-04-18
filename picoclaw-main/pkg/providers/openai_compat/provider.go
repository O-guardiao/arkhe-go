package openai_compat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/providers/common"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/providers/protocoltypes"
)

type (
	ToolCall               = protocoltypes.ToolCall
	FunctionCall           = protocoltypes.FunctionCall
	LLMResponse            = protocoltypes.LLMResponse
	UsageInfo              = protocoltypes.UsageInfo
	Message                = protocoltypes.Message
	ToolDefinition         = protocoltypes.ToolDefinition
	ToolFunctionDefinition = protocoltypes.ToolFunctionDefinition
	ExtraContent           = protocoltypes.ExtraContent
	GoogleExtra            = protocoltypes.GoogleExtra
	ReasoningDetail        = protocoltypes.ReasoningDetail
)

type Provider struct {
	apiKey         string
	apiBase        string
	maxTokensField string // Field name for max tokens (e.g., "max_completion_tokens" for o1/glm models)
	httpClient     *http.Client
	extraBody      map[string]any // Additional fields to inject into request body
	customHeaders  map[string]string
	userAgent      string
}

type Option func(*Provider)

const defaultRequestTimeout = common.DefaultRequestTimeout

var stripModelPrefixProviders = map[string]struct{}{
	"litellm":    {},
	"venice":     {},
	"moonshot":   {},
	"nvidia":     {},
	"groq":       {},
	"ollama":     {},
	"deepseek":   {},
	"google":     {},
	"openrouter": {},
	"zhipu":      {},
	"mistral":    {},
	"vivgrid":    {},
	"minimax":    {},
	"novita":     {},
	"lmstudio":   {},
}

func WithMaxTokensField(maxTokensField string) Option {
	return func(p *Provider) {
		p.maxTokensField = maxTokensField
	}
}

func WithUserAgent(userAgent string) Option {
	return func(p *Provider) {
		p.userAgent = userAgent
	}
}

func WithRequestTimeout(timeout time.Duration) Option {
	return func(p *Provider) {
		if timeout > 0 {
			p.httpClient.Timeout = timeout
		}
	}
}

func WithExtraBody(extraBody map[string]any) Option {
	return func(p *Provider) {
		p.extraBody = extraBody
	}
}

func WithCustomHeaders(customHeaders map[string]string) Option {
	return func(p *Provider) {
		p.customHeaders = customHeaders
	}
}

func NewProvider(apiKey, apiBase, proxy string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:     apiKey,
		apiBase:    strings.TrimRight(apiBase, "/"),
		httpClient: common.NewHTTPClient(proxy),
	}

	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}

	return p
}

func NewProviderWithMaxTokensField(apiKey, apiBase, proxy, maxTokensField string) *Provider {
	return NewProvider(apiKey, apiBase, proxy, WithMaxTokensField(maxTokensField))
}

func NewProviderWithMaxTokensFieldAndTimeout(
	apiKey, apiBase, proxy, maxTokensField string,
	requestTimeoutSeconds int,
) *Provider {
	return NewProvider(
		apiKey,
		apiBase,
		proxy,
		WithMaxTokensField(maxTokensField),
		WithRequestTimeout(time.Duration(requestTimeoutSeconds)*time.Second),
	)
}

// buildRequestBody constructs the common request body for Chat and ChatStream.
func (p *Provider) buildRequestBody(
	messages []Message, tools []ToolDefinition, model string, options map[string]any,
) map[string]any {
	model = normalizeModel(model, p.apiBase)

	requestBody := map[string]any{
		"model":    model,
		"messages": common.SerializeMessages(messages),
	}

	// Chat Completions API does NOT support web_search_preview tool type.
	// Native search is only valid on the Responses API (codex_provider).
	// Always ignore native_search here to prevent 400 errors.
	if len(tools) > 0 {
		requestBody["tools"] = buildToolsList(tools, false)
		requestBody["tool_choice"] = "auto"
	}

	if maxTokens, ok := common.AsInt(options["max_tokens"]); ok {
		fieldName := p.maxTokensField
		if fieldName == "" {
			lowerModel := strings.ToLower(model)
			if strings.Contains(lowerModel, "glm") || strings.Contains(lowerModel, "o1") ||
				strings.Contains(lowerModel, "gpt-5") {
				fieldName = "max_completion_tokens"
			} else {
				fieldName = "max_tokens"
			}
		}
		requestBody[fieldName] = maxTokens
	}

	if temperature, ok := common.AsFloat(options["temperature"]); ok {
		lowerModel := strings.ToLower(model)
		if strings.Contains(lowerModel, "kimi") && strings.Contains(lowerModel, "k2") {
			requestBody["temperature"] = 1.0
		} else {
			requestBody["temperature"] = temperature
		}
	}

	// Prompt caching: pass a stable cache key so OpenAI can bucket requests
	// with the same key and reuse prefix KV cache across calls.
	// Prompt caching is only supported by OpenAI-native endpoints.
	// Non-OpenAI providers reject unknown fields with 422 errors.
	if cacheKey, ok := options["prompt_cache_key"].(string); ok && cacheKey != "" {
		if supportsPromptCacheKey(p.apiBase) {
			requestBody["prompt_cache_key"] = cacheKey
		}
	}

	// Extended thinking / reasoning support for OpenAI-compatible providers.
	// Translates the unified thinking_level (off/low/medium/high/xhigh/adaptive)
	// into provider-specific parameters:
	//   - Qwen/DashScope:   enable_thinking + thinking_budget
	//   - OpenAI o-series:  reasoning_effort (low/medium/high)
	//   - DeepSeek:         thinking is implicit in R1 models; no parameter needed
	//   - Generic:          reasoning_effort as best-effort passthrough
	if level, ok := options["thinking_level"].(string); ok && level != "" && level != "off" {
		applyOpenAICompatThinking(requestBody, model, p.apiBase, level)
	}

	// Merge extra body fields configured per-provider/model.
	// These are injected last so they take precedence over defaults.
	maps.Copy(requestBody, p.extraBody)

	return requestBody
}

func (p *Provider) applyCustomHeaders(req *http.Request) {
	for k, v := range p.customHeaders {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}
}

func (p *Provider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	if p.apiBase == "" {
		return nil, fmt.Errorf("API base not configured")
	}

	requestBody := p.buildRequestBody(messages, tools, model, options)

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.userAgent != "" {
		req.Header.Set("User-Agent", p.userAgent)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	p.applyCustomHeaders(req)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, common.HandleErrorResponse(resp, p.apiBase)
	}

	return common.ReadAndParseResponse(resp, p.apiBase)
}

// ChatStream implements streaming via OpenAI-compatible SSE (stream: true).
// onChunk receives the accumulated text so far on each text delta.
func (p *Provider) ChatStream(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
	onChunk func(accumulated string),
) (*LLMResponse, error) {
	if p.apiBase == "" {
		return nil, fmt.Errorf("API base not configured")
	}

	requestBody := p.buildRequestBody(messages, tools, model, options)
	requestBody["stream"] = true

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.userAgent != "" {
		req.Header.Set("User-Agent", p.userAgent)
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	p.applyCustomHeaders(req)

	// Use a client without Timeout for streaming — the http.Client.Timeout covers
	// the entire request lifecycle including body reads, which would kill long streams.
	// Context cancellation still provides the safety net.
	streamClient := &http.Client{Transport: p.httpClient.Transport}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, common.HandleErrorResponse(resp, p.apiBase)
	}

	return parseStreamResponse(ctx, resp.Body, onChunk)
}

// parseStreamResponse parses an OpenAI-compatible SSE stream.
func parseStreamResponse(
	ctx context.Context,
	reader io.Reader,
	onChunk func(accumulated string),
) (*LLMResponse, error) {
	var textContent strings.Builder
	var finishReason string
	var usage *UsageInfo

	// Tool call assembly: OpenAI streams tool calls as incremental deltas
	type toolAccum struct {
		id       string
		name     string
		argsJSON strings.Builder
	}
	activeTools := map[int]*toolAccum{}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 1MB initial, 10MB max
	for scanner.Scan() {
		// Check for context cancellation between chunks
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function *struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *UsageInfo `json:"usage"`
		}

		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed chunks
		}

		if chunk.Usage != nil {
			usage = chunk.Usage
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]

		// Accumulate text content
		if choice.Delta.Content != "" {
			textContent.WriteString(choice.Delta.Content)
			if onChunk != nil {
				onChunk(textContent.String())
			}
		}

		// Accumulate tool call deltas
		for _, tc := range choice.Delta.ToolCalls {
			acc, ok := activeTools[tc.Index]
			if !ok {
				acc = &toolAccum{}
				activeTools[tc.Index] = acc
			}
			if tc.ID != "" {
				acc.id = tc.ID
			}
			if tc.Function != nil {
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					acc.argsJSON.WriteString(tc.Function.Arguments)
				}
			}
		}

		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("streaming read error: %w", err)
	}

	// Assemble tool calls from accumulated deltas
	var toolCalls []ToolCall
	for i := 0; i < len(activeTools); i++ {
		acc, ok := activeTools[i]
		if !ok {
			continue
		}
		args := make(map[string]any)
		raw := acc.argsJSON.String()
		if raw != "" {
			if err := json.Unmarshal([]byte(raw), &args); err != nil {
				log.Printf("openai_compat stream: failed to decode tool call arguments for %q: %v", acc.name, err)
				args["raw"] = raw
			}
		}
		toolCalls = append(toolCalls, ToolCall{
			ID:        acc.id,
			Name:      acc.name,
			Arguments: args,
		})
	}

	if finishReason == "" {
		finishReason = "stop"
	}

	return &LLMResponse{
		Content:      textContent.String(),
		ToolCalls:    toolCalls,
		FinishReason: finishReason,
		Usage:        usage,
	}, nil
}

func normalizeModel(model, apiBase string) string {
	before, after, ok := strings.Cut(model, "/")
	if !ok {
		return model
	}

	if strings.Contains(strings.ToLower(apiBase), "openrouter.ai") {
		return model
	}

	prefix := strings.ToLower(before)
	if _, ok := stripModelPrefixProviders[prefix]; ok {
		return after
	}

	return model
}

func buildToolsList(tools []ToolDefinition, nativeSearch bool) []any {
	result := make([]any, 0, len(tools)+1)
	for _, t := range tools {
		if nativeSearch && strings.EqualFold(t.Function.Name, "web_search") {
			continue
		}
		result = append(result, t)
	}
	if nativeSearch {
		result = append(result, map[string]any{"type": "web_search_preview"})
	}
	return result
}

// SupportsNativeSearch returns false because the openai_compat provider uses
// the Chat Completions API (/chat/completions), which does NOT support
// web_search_preview as a tool type — only 'function' and 'custom' are valid.
// Native web search is handled exclusively by providers that use the
// Responses API (codex_provider, openai_responses_common).
func (p *Provider) SupportsNativeSearch() bool {
	return false
}

// supportsPromptCacheKey reports whether the given API base is known to
// support the prompt_cache_key request field. Currently only OpenAI's own
// API and Azure OpenAI support this. All other OpenAI-compatible providers
// (Mistral, Gemini, DeepSeek, Groq, etc.) reject unknown fields with 422 errors.
func supportsPromptCacheKey(apiBase string) bool {
	u, err := url.Parse(apiBase)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "api.openai.com" || strings.HasSuffix(host, ".openai.azure.com")
}

// applyOpenAICompatThinking injects provider-specific extended thinking
// parameters into the request body based on the unified thinking_level.
//
// Provider mappings:
//
//	Qwen/DashScope:  enable_thinking=true + thinking_budget=N
//	OpenAI (o-series): reasoning_effort="low"|"medium"|"high"
//	DeepSeek R1:     thinking is implicit; no parameter injected
//	Generic:         reasoning_effort as best-effort passthrough
func applyOpenAICompatThinking(body map[string]any, model, apiBase, level string) {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" || level == "off" {
		return
	}

	lowerBase := strings.ToLower(apiBase)
	lowerModel := strings.ToLower(model)

	switch {
	case isQwenEndpoint(lowerBase) || isQwenModel(lowerModel):
		body["enable_thinking"] = true
		if budget := qwenThinkingBudget(level); budget > 0 {
			body["thinking_budget"] = budget
		}
		log.Printf("openai_compat: thinking enabled for Qwen (level=%s)", level)

	case isDeepSeekEndpoint(lowerBase) || isDeepSeekModel(lowerModel):
		// DeepSeek R1 models think implicitly; no request param to control it.
		// DeepSeek V3 and later may accept reasoning_effort but docs don't
		// guarantee it. Skip to avoid 422 errors.
		log.Printf("openai_compat: thinking level=%s noted for DeepSeek (implicit reasoning)", level)

	case isOpenAIEndpoint(lowerBase):
		// OpenAI o-series models use reasoning_effort (low/medium/high).
		if effort := mapReasoningEffort(level); effort != "" {
			body["reasoning_effort"] = effort
		}
		log.Printf("openai_compat: reasoning_effort=%s for OpenAI o-series", mapReasoningEffort(level))

	default:
		// Generic OpenAI-compatible: try reasoning_effort as best-effort.
		// Many providers silently ignore unknown fields; those that don't
		// will return a 422, which the retry logic can handle.
		if effort := mapReasoningEffort(level); effort != "" {
			body["reasoning_effort"] = effort
		}
		log.Printf("openai_compat: reasoning_effort=%s for generic provider (best-effort)", mapReasoningEffort(level))
	}
}

// qwenThinkingBudget maps unified thinking levels to Qwen thinking_budget tokens.
// Values are chosen to match the Anthropic budget ladder proportionally,
// scaled to Qwen's default max thinking length.
func qwenThinkingBudget(level string) int {
	switch level {
	case "low":
		return 4096
	case "medium":
		return 16384
	case "high":
		return 32768
	case "xhigh":
		return 65536
	case "adaptive":
		// Qwen doesn't have an "adaptive" mode; use xhigh budget
		// and let the model decide.
		return 65536
	default:
		return 0
	}
}

// mapReasoningEffort maps unified thinking levels to OpenAI reasoning_effort values.
func mapReasoningEffort(level string) string {
	switch level {
	case "low":
		return "low"
	case "medium":
		return "medium"
	case "high", "xhigh", "adaptive":
		return "high"
	default:
		return ""
	}
}

func isQwenEndpoint(lowerBase string) bool {
	return strings.Contains(lowerBase, "dashscope")
}

func isQwenModel(lowerModel string) bool {
	return strings.HasPrefix(lowerModel, "qwen")
}

func isDeepSeekEndpoint(lowerBase string) bool {
	return strings.Contains(lowerBase, "deepseek")
}

func isDeepSeekModel(lowerModel string) bool {
	return strings.HasPrefix(lowerModel, "deepseek")
}

func isOpenAIEndpoint(lowerBase string) bool {
	u, err := url.Parse(lowerBase)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "api.openai.com" || strings.HasSuffix(host, ".openai.azure.com")
}
