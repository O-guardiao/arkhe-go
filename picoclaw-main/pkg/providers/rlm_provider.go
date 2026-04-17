package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	rlm "github.com/O-guardiao/arkhe-go/rlm-go"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
)

const (
	rlmOptionKeyAgentID          = "rlm_agent_id"
	rlmOptionKeySessionKey       = "rlm_session_key"
	rlmOptionKeyChannel          = "rlm_channel"
	rlmOptionKeyChatID           = "rlm_chat_id"
	rlmOptionKeyMessageID        = "rlm_message_id"
	rlmOptionKeyReplyToMessageID = "rlm_reply_to_message_id"
)

type rlmEngineFactory func(cfg rlm.Config) (*rlm.RLM, error)

type rlmSessionState struct {
	mu              sync.Mutex
	metaMu          sync.RWMutex
	ctxMu           sync.RWMutex
	engine          *rlm.RLM
	engineSignature string
	currentMeta     ToolCallContext
	currentCtx      context.Context
}

func (s *rlmSessionState) setMeta(meta ToolCallContext) {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	s.currentMeta = meta
}

func (s *rlmSessionState) meta() ToolCallContext {
	s.metaMu.RLock()
	defer s.metaMu.RUnlock()
	return s.currentMeta
}

func (s *rlmSessionState) setContext(ctx context.Context) {
	s.ctxMu.Lock()
	defer s.ctxMu.Unlock()
	if ctx == nil {
		s.currentCtx = context.Background()
		return
	}
	s.currentCtx = ctx
}

func (s *rlmSessionState) requestContext() context.Context {
	s.ctxMu.RLock()
	defer s.ctxMu.RUnlock()
	if s.currentCtx != nil {
		return s.currentCtx
	}
	return context.Background()
}

func (s *rlmSessionState) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.engine != nil {
		_ = s.engine.Close()
		s.engine = nil
		s.engineSignature = ""
	}
	s.setContext(context.Background())
}

type rlmToolSignature struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type rlmEngineSignature struct {
	Backend                string             `json:"backend"`
	BackendKwargs          map[string]any     `json:"backend_kwargs,omitempty"`
	Environment            string             `json:"environment"`
	MaxDepth               int                `json:"max_depth"`
	MaxIterations          int                `json:"max_iterations"`
	MaxTimeoutNanos        int64              `json:"max_timeout_nanos"`
	MaxErrors              int                `json:"max_errors"`
	MaxTokens              int                `json:"max_tokens"`
	Persistent             bool               `json:"persistent"`
	Compaction             bool               `json:"compaction"`
	CompactionThresholdPct float64            `json:"compaction_threshold_pct"`
	MaxConcurrentSubcalls  int                `json:"max_concurrent_subcalls"`
	Tools                  []rlmToolSignature `json:"tools,omitempty"`
}

// RLMProvider bridges PicoClaw's agent loop to the local Go RLM runtime.
// The provider remains fully local and exposes the active PicoClaw tools back
// into the RLM REPL as callable Go functions.
type RLMProvider struct {
	modelCfg      config.ModelConfig
	defaultModel  string
	bindingMu     sync.RWMutex
	runtime       AgentRuntimeBinding
	newEngineFunc rlmEngineFactory
	sessions      sync.Map
	closed        atomic.Bool
}

func NewRLMProvider(cfg *config.ModelConfig) (*RLMProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("model is required")
	}

	_, modelID := ExtractProtocol(cfg.Model)
	if _, _, _, err := resolveRLMBackend(modelID, cfg); err != nil {
		return nil, err
	}

	clone := *cfg
	return &RLMProvider{
		modelCfg:      clone,
		defaultModel:  modelID,
		newEngineFunc: rlm.New,
	}, nil
}

func (p *RLMProvider) BindRuntime(binding AgentRuntimeBinding) {
	p.bindingMu.Lock()
	defer p.bindingMu.Unlock()
	p.runtime = binding
}

func (p *RLMProvider) Close() {
	if p.closed.Swap(true) {
		return
	}
	p.sessions.Range(func(key, value any) bool {
		if state, ok := value.(*rlmSessionState); ok {
			state.close()
		}
		p.sessions.Delete(key)
		return true
	})
}

func (p *RLMProvider) GetDefaultModel() string {
	return p.defaultModel
}

func (p *RLMProvider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	if p.closed.Load() {
		return nil, fmt.Errorf("rlm provider is closed")
	}
	if strings.TrimSpace(model) == "" {
		model = p.defaultModel
	}

	meta := rlmToolCallContextFromOptions(options)
	if p.shouldPersistSession(options) {
		return p.chatWithSession(ctx, messages, tools, model, options, meta)
	}

	completion, resolvedModel, err := p.runEphemeralCompletion(ctx, messages, tools, model, options, meta)
	if err != nil {
		return nil, err
	}

	return completionToResponse(completion, resolvedModel), nil
}

func (p *RLMProvider) buildCustomTools(
	toolDefs []ToolDefinition,
	metaProvider func() ToolCallContext,
	ctxProvider func() context.Context,
) map[string]any {
	p.bindingMu.RLock()
	binding := p.runtime
	p.bindingMu.RUnlock()

	out := make(map[string]any, len(toolDefs)+1)
	schemas := make(map[string]any, len(toolDefs))

	for _, toolDef := range toolDefs {
		name := strings.TrimSpace(toolDef.Function.Name)
		if name == "" {
			continue
		}

		schemas[name] = toolDef.Function.Parameters
		description := toolDef.Function.Description
		if schemaJSON, err := json.Marshal(toolDef.Function.Parameters); err == nil {
			description = strings.TrimSpace(description)
			if description != "" {
				description += " "
			}
			description += fmt.Sprintf(
				"Call with a Go map like map[string]any{...} or a JSON string. JSON schema: %s",
				string(schemaJSON),
			)
		}

		toolName := name
		out[toolName] = map[string]any{
			"description": description,
			"tool": func(input any) string {
				if binding.ExecuteTool == nil {
					return fmt.Sprintf("Error: tool runtime is not bound for %q", toolName)
				}
				args, err := normalizeRLMToolArgs(input)
				if err != nil {
					return "Error: " + err.Error()
				}
				// Guard against excessively large arguments that could indicate
				// prompt injection or resource abuse through the tool bridge.
				if raw, marshalErr := json.Marshal(args); marshalErr == nil && len(raw) > 512*1024 {
					return fmt.Sprintf("Error: tool %q arguments exceed 512KB size limit", toolName)
				}
				// Apply a per-tool timeout to prevent a single tool call from
				// blocking the entire RLM loop indefinitely.
				toolCtx := ctxProvider()
				if toolCtx == nil {
					toolCtx = context.Background()
				}
				toolCtx, cancel := context.WithTimeout(toolCtx, 120*time.Second)
				defer cancel()
				result := binding.ExecuteTool(toolCtx, toolName, args, metaProvider())
				if strings.TrimSpace(result.Content) == "" {
					if result.IsError {
						return fmt.Sprintf("Error: tool %q failed without output", toolName)
					}
					return fmt.Sprintf("Tool %q completed", toolName)
				}
				return result.Content
			},
		}
	}

	if len(schemas) > 0 {
		out["tool_schemas"] = map[string]any{
			"description": "Map of PicoClaw tool JSON schemas keyed by tool name.",
			"tool":        schemas,
		}
	}

	return out
}

func (p *RLMProvider) chatWithSession(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
	meta ToolCallContext,
) (*LLMResponse, error) {
	cacheKey := rlmSessionCacheKey(meta)
	if cacheKey == "" {
		completion, resolvedModel, err := p.runEphemeralCompletion(ctx, messages, tools, model, options, meta)
		if err != nil {
			return nil, err
		}
		return completionToResponse(completion, resolvedModel), nil
	}

	stateAny, _ := p.sessions.LoadOrStore(cacheKey, &rlmSessionState{})
	state := stateAny.(*rlmSessionState)
	state.setMeta(meta)
	state.setContext(ctx)
	defer state.setContext(context.Background())

	engineCfg, engineSignature, resolvedModel, err := p.buildEngineConfig(
		tools,
		options,
		state.meta,
		state.requestContext,
		model,
		true,
	)
	if err != nil {
		return nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.engine == nil || state.engineSignature != engineSignature {
		if state.engine != nil {
			_ = state.engine.Close()
		}
		engine, engineErr := p.newEngineFunc(engineCfg)
		if engineErr != nil {
			return nil, engineErr
		}
		state.engine = engine
		state.engineSignature = engineSignature
	}

	completion, err := state.engine.Completion(picoMessagesToRLMContext(messages), lastUserContent(messages))
	if err != nil {
		return nil, err
	}

	return completionToResponse(completion, resolvedModel), nil
}

func (p *RLMProvider) runEphemeralCompletion(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
	meta ToolCallContext,
) (rlm.RLMChatCompletion, string, error) {
	metaProvider := func() ToolCallContext { return meta }
	ctxProvider := func() context.Context {
		if ctx != nil {
			return ctx
		}
		return context.Background()
	}
	engineCfg, _, resolvedModel, err := p.buildEngineConfig(tools, options, metaProvider, ctxProvider, model, false)
	if err != nil {
		return rlm.RLMChatCompletion{}, "", err
	}
	engine, err := p.newEngineFunc(engineCfg)
	if err != nil {
		return rlm.RLMChatCompletion{}, "", err
	}
	defer engine.Close()

	completion, err := engine.Completion(picoMessagesToRLMContext(messages), lastUserContent(messages))
	if err != nil {
		return rlm.RLMChatCompletion{}, "", err
	}
	return completion, resolvedModel, nil
}

func (p *RLMProvider) buildEngineConfig(
	toolDefs []ToolDefinition,
	options map[string]any,
	metaProvider func() ToolCallContext,
	ctxProvider func() context.Context,
	model string,
	persistent bool,
) (rlm.Config, string, string, error) {
	backend, backendKwargs, resolvedModel, err := resolveRLMBackend(model, &p.modelCfg)
	if err != nil {
		return rlm.Config{}, "", "", err
	}

	customTools := p.buildCustomTools(toolDefs, metaProvider, ctxProvider)
	p.bindingMu.RLock()
	binding := p.runtime
	p.bindingMu.RUnlock()
	environmentKwargs := map[string]any{}
	if workspace := strings.TrimSpace(binding.Workspace); workspace != "" {
		environmentKwargs["working_dir"] = workspace
		environmentKwargs["write_context_files"] = false
	}
	engineCfg := rlm.Config{
		Backend:                backend,
		BackendKwargs:          backendKwargs,
		ClientFactory:          newRLMEmbeddedClient,
		Environment:            "local",
		EnvironmentKwargs:      environmentKwargs,
		MaxDepth:               resolveNestedInt(p.modelCfg.ExtraBody, []string{"rlm", "max_depth"}, 2),
		MaxIterations:          resolveNestedInt(p.modelCfg.ExtraBody, []string{"rlm", "max_iterations"}, 30),
		MaxTimeout:             resolveRLMTimeout(p.modelCfg, options),
		MaxErrors:              resolveNestedInt(p.modelCfg.ExtraBody, []string{"rlm", "max_errors"}, 3),
		MaxTokens:              optionAsInt(options, "max_tokens"),
		Persistent:             persistent,
		CustomTools:            customTools,
		CustomSubTools:         customTools,
		Compaction:             resolveNestedBool(p.modelCfg.ExtraBody, []string{"rlm", "compaction"}, true),
		CompactionThresholdPct: resolveNestedFloat(p.modelCfg.ExtraBody, []string{"rlm", "compaction_threshold_pct"}, 0.85),
		MaxConcurrentSubcalls:  resolveNestedInt(p.modelCfg.ExtraBody, []string{"rlm", "max_concurrent_subcalls"}, 4),
	}

	engineSignature, err := buildRLMEngineSignature(engineCfg, toolDefs)
	if err != nil {
		return rlm.Config{}, "", "", err
	}

	return engineCfg, engineSignature, resolvedModel, nil
}

func buildRLMEngineSignature(engineCfg rlm.Config, toolDefs []ToolDefinition) (string, error) {
	toolSnapshot := make([]rlmToolSignature, 0, len(toolDefs))
	for _, toolDef := range toolDefs {
		name := strings.TrimSpace(toolDef.Function.Name)
		if name == "" {
			continue
		}
		toolSnapshot = append(toolSnapshot, rlmToolSignature{
			Name:        name,
			Description: strings.TrimSpace(toolDef.Function.Description),
			Parameters:  toolDef.Function.Parameters,
		})
	}
	sort.Slice(toolSnapshot, func(i, j int) bool {
		return toolSnapshot[i].Name < toolSnapshot[j].Name
	})

	payload, err := json.Marshal(rlmEngineSignature{
		Backend:                engineCfg.Backend,
		BackendKwargs:          engineCfg.BackendKwargs,
		Environment:            engineCfg.Environment,
		MaxDepth:               engineCfg.MaxDepth,
		MaxIterations:          engineCfg.MaxIterations,
		MaxTimeoutNanos:        engineCfg.MaxTimeout.Nanoseconds(),
		MaxErrors:              engineCfg.MaxErrors,
		MaxTokens:              engineCfg.MaxTokens,
		Persistent:             engineCfg.Persistent,
		Compaction:             engineCfg.Compaction,
		CompactionThresholdPct: engineCfg.CompactionThresholdPct,
		MaxConcurrentSubcalls:  engineCfg.MaxConcurrentSubcalls,
		Tools:                  toolSnapshot,
	})
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func completionToResponse(completion rlm.RLMChatCompletion, resolvedModel string) *LLMResponse {
	usage := &UsageInfo{
		PromptTokens:     completion.UsageSummary.TotalInputTokens(),
		CompletionTokens: completion.UsageSummary.TotalOutputTokens(),
		TotalTokens:      completion.UsageSummary.TotalInputTokens() + completion.UsageSummary.TotalOutputTokens(),
	}
	if usage.TotalTokens == 0 && usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		usage = nil
	}

	resp := &LLMResponse{
		Content:      completion.Response,
		FinishReason: "stop",
		Usage:        usage,
		Reasoning:    fmt.Sprintf("rlm local runtime completed with backend %s", resolvedModel),
	}

	// Propagate RLM trajectory as ReasoningDetails so the agent loop
	// (and hooks like memory_extractor) can observe what the RLM engine
	// actually did: iterations, code executed, sub-calls, etc.
	resp.ReasoningDetails = extractRLMTrajectory(completion.Metadata)

	return resp
}

// extractRLMTrajectory converts the RLM engine's Metadata (iterations, code
// blocks, sub-calls) into compact ReasoningDetail entries. Only concise
// summaries are kept — full prompts are omitted to save tokens.
func extractRLMTrajectory(metadata map[string]any) []ReasoningDetail {
	if len(metadata) == 0 {
		return nil
	}

	var details []ReasoningDetail

	// Extract run_metadata summary.
	if rm, ok := metadata["run_metadata"]; ok {
		if rmMap, ok := rm.(map[string]any); ok {
			summary := formatRLMRunMetadata(rmMap)
			if summary != "" {
				details = append(details, ReasoningDetail{
					Type:   "rlm_run_metadata",
					Format: "text",
					Text:   summary,
				})
			}
		}
	}

	// Extract iterations.
	itersRaw, ok := metadata["iterations"]
	if !ok {
		return details
	}

	iters, ok := itersRaw.([]any)
	if !ok {
		return details
	}

	for i, iterRaw := range iters {
		iterMap, ok := iterRaw.(map[string]any)
		if !ok {
			continue
		}
		summary := formatRLMIteration(i+1, iterMap)
		if summary == "" {
			continue
		}
		details = append(details, ReasoningDetail{
			Type:   "rlm_iteration",
			Format: "text",
			Index:  i,
			Text:   summary,
		})
	}

	return details
}

// formatRLMRunMetadata summarizes the run_metadata block.
func formatRLMRunMetadata(rm map[string]any) string {
	var parts []string
	if model, ok := rm["root_model"].(string); ok && model != "" {
		parts = append(parts, "model="+model)
	}
	if maxIter, ok := rm["max_iterations"]; ok {
		parts = append(parts, fmt.Sprintf("max_iterations=%v", maxIter))
	}
	if maxDepth, ok := rm["max_depth"]; ok {
		parts = append(parts, fmt.Sprintf("max_depth=%v", maxDepth))
	}
	if backend, ok := rm["backend"].(string); ok && backend != "" {
		parts = append(parts, "backend="+backend)
	}
	if len(parts) == 0 {
		return ""
	}
	return "RLM run: " + strings.Join(parts, ", ")
}

// formatRLMIteration produces a concise summary of a single RLM iteration.
func formatRLMIteration(num int, iter map[string]any) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Iteration %d", num)

	// Summarize code blocks.
	if codeBlocks, ok := iter["code_blocks"].([]any); ok && len(codeBlocks) > 0 {
		fmt.Fprintf(&sb, " [%d code block(s)", len(codeBlocks))
		for _, cbRaw := range codeBlocks {
			cb, ok := cbRaw.(map[string]any)
			if !ok {
				continue
			}
			code, _ := cb["code"].(string)
			if len(code) > 200 {
				code = code[:200] + "..."
			}
			if code != "" {
				fmt.Fprintf(&sb, ": %s", strings.ReplaceAll(code, "\n", "; "))
			}
			// Summarize result.
			if result, ok := cb["result"].(map[string]any); ok {
				if stdout, ok := result["stdout"].(string); ok && stdout != "" {
					if len(stdout) > 100 {
						stdout = stdout[:100] + "..."
					}
					fmt.Fprintf(&sb, " → stdout: %s", strings.TrimSpace(stdout))
				}
				if stderr, ok := result["stderr"].(string); ok && stderr != "" {
					if len(stderr) > 100 {
						stderr = stderr[:100] + "..."
					}
					fmt.Fprintf(&sb, " → stderr: %s", strings.TrimSpace(stderr))
				}
				// Count RLM sub-calls.
				if rlmCalls, ok := result["rlm_calls"].([]any); ok && len(rlmCalls) > 0 {
					fmt.Fprintf(&sb, " [%d sub-call(s)]", len(rlmCalls))
				}
			}
		}
		sb.WriteString("]")
	}

	// Final answer?
	if fa, ok := iter["final_answer"].(string); ok && fa != "" {
		if len(fa) > 150 {
			fa = fa[:150] + "..."
		}
		fmt.Fprintf(&sb, " → answer: %s", fa)
	}

	// Iteration time.
	if t, ok := iter["iteration_time"].(float64); ok && t > 0 {
		fmt.Fprintf(&sb, " (%.1fs)", t)
	}

	return sb.String()
}

func (p *RLMProvider) shouldPersistSession(options map[string]any) bool {
	if optionAsString(options, rlmOptionKeySessionKey) == "" {
		return false
	}
	return resolveNestedBool(p.modelCfg.ExtraBody, []string{"rlm", "persistent_session"}, true)
}

func rlmSessionCacheKey(meta ToolCallContext) string {
	if strings.TrimSpace(meta.SessionKey) == "" {
		return ""
	}
	if strings.TrimSpace(meta.AgentID) == "" {
		return meta.SessionKey
	}
	return meta.AgentID + "::" + meta.SessionKey
}

func picoMessagesToRLMContext(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		item := map[string]any{
			"role":    msg.Role,
			"content": msg.Content,
		}
		if len(msg.Media) > 0 {
			item["media"] = append([]string(nil), msg.Media...)
		}
		if msg.ReasoningContent != "" {
			item["reasoning_content"] = msg.ReasoningContent
		}
		if len(msg.ToolCalls) > 0 {
			toolCalls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				call := map[string]any{
					"id":   tc.ID,
					"name": tc.Name,
				}
				if len(tc.Arguments) > 0 {
					call["arguments"] = tc.Arguments
				}
				if tc.Function != nil {
					call["function"] = map[string]any{
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
					}
				}
				toolCalls = append(toolCalls, call)
			}
			item["tool_calls"] = toolCalls
		}
		if msg.ToolCallID != "" {
			item["tool_call_id"] = msg.ToolCallID
		}
		out = append(out, item)
	}
	return out
}

func lastUserContent(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	return ""
}

func normalizeRLMToolArgs(input any) (map[string]any, error) {
	switch value := input.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return value, nil
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return map[string]any{}, nil
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
			return nil, fmt.Errorf("tool arguments must be a map or JSON object string: %w", err)
		}
		return decoded, nil
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("unsupported tool arguments type %T", input)
		}
		var decoded map[string]any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, fmt.Errorf("tool arguments must decode to an object, got %T", input)
		}
		return decoded, nil
	}
}

func rlmToolCallContextFromOptions(options map[string]any) ToolCallContext {
	return ToolCallContext{
		AgentID:          optionAsString(options, rlmOptionKeyAgentID),
		SessionKey:       optionAsString(options, rlmOptionKeySessionKey),
		Channel:          optionAsString(options, rlmOptionKeyChannel),
		ChatID:           optionAsString(options, rlmOptionKeyChatID),
		MessageID:        optionAsString(options, rlmOptionKeyMessageID),
		ReplyToMessageID: optionAsString(options, rlmOptionKeyReplyToMessageID),
	}
}

func optionAsString(options map[string]any, key string) string {
	if options == nil {
		return ""
	}
	raw, ok := options[key]
	if !ok {
		return ""
	}
	text, _ := raw.(string)
	return strings.TrimSpace(text)
}

func optionAsInt(options map[string]any, key string) int {
	if options == nil {
		return 0
	}
	switch value := options[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func resolveRLMTimeout(modelCfg config.ModelConfig, options map[string]any) time.Duration {
	if modelCfg.RequestTimeout > 0 {
		return time.Duration(modelCfg.RequestTimeout) * time.Second
	}
	if seconds := resolveNestedInt(modelCfg.ExtraBody, []string{"rlm", "max_timeout_sec"}, 0); seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if seconds := optionAsInt(options, "timeout"); seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return 0
}

func resolveNestedInt(root map[string]any, path []string, fallback int) int {
	value, ok := resolveNestedValue(root, path)
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func resolveNestedFloat(root map[string]any, path []string, fallback float64) float64 {
	value, ok := resolveNestedValue(root, path)
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return fallback
	}
}

func resolveNestedBool(root map[string]any, path []string, fallback bool) bool {
	value, ok := resolveNestedValue(root, path)
	if !ok {
		return fallback
	}
	typed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return typed
}

func resolveNestedValue(root map[string]any, path []string) (any, bool) {
	if len(path) == 0 || root == nil {
		return nil, false
	}
	var current any = root
	for _, key := range path {
		nextMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := nextMap[key]
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func resolveRLMBackend(model string, cfg *config.ModelConfig) (string, map[string]any, string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", nil, "", fmt.Errorf("rlm provider requires an underlying local model")
	}

	backendProtocol := "openai"
	backendModel := model
	if strings.Contains(model, "/") {
		backendProtocol, backendModel = ExtractProtocol(model)
	} else if strings.TrimSpace(cfg.APIBase) == "" {
		return "", nil, "", fmt.Errorf("rlm/openai models without nested provider require a local api_base")
	}

	apiBase := strings.TrimSpace(cfg.APIBase)
	switch NormalizeProvider(backendProtocol) {
	case "openai":
		if apiBase == "" {
			return "", nil, "", fmt.Errorf("rlm/openai requires a local api_base")
		}
	case "vllm":
		if apiBase == "" {
			apiBase = "http://127.0.0.1:8000/v1"
		}
	case "ollama":
		if apiBase == "" {
			apiBase = "http://127.0.0.1:11434/v1"
		}
	case "lmstudio":
		if apiBase == "" {
			apiBase = "http://127.0.0.1:1234/v1"
		}
	case "litellm":
		if apiBase == "" {
			apiBase = "http://127.0.0.1:4000/v1"
		}
	default:
		return "", nil, "", fmt.Errorf("rlm only supports local openai-compatible backends, got %q", backendProtocol)
	}

	if !isLocalAPIBase(apiBase) {
		return "", nil, "", fmt.Errorf("rlm provider only allows local api_base values, got %q", apiBase)
	}

	backend := "openai"
	if NormalizeProvider(backendProtocol) == "vllm" {
		backend = "vllm"
	}

	backendKwargs := map[string]any{
		"model_name": backendModel,
		"base_url":   strings.TrimRight(apiBase, "/"),
		"api_key":    cfg.APIKey(),
	}
	if cfg.RequestTimeout > 0 {
		backendKwargs["timeout"] = cfg.RequestTimeout
	}
	if len(cfg.CustomHeaders) > 0 {
		headers := make(map[string]any, len(cfg.CustomHeaders))
		for key, value := range cfg.CustomHeaders {
			headers[key] = value
		}
		backendKwargs["default_headers"] = headers
	}

	return backend, backendKwargs, backendModel, nil
}

func isLocalAPIBase(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
