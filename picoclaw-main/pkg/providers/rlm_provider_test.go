package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
)

func TestRLMProviderChatExecutesBoundTools(t *testing.T) {
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		var requestBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		response := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "```repl\nanswer := echo_tool(map[string]any{\"value\":\"hi\"})\n```\nFINAL_VAR(answer)",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     11,
				"completion_tokens": 7,
				"total_tokens":      18,
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider, err := NewRLMProvider(&config.ModelConfig{
		ModelName: "local-rlm",
		Model:     "rlm/lmstudio/local-model",
		APIBase:   server.URL,
	})
	if err != nil {
		t.Fatalf("NewRLMProvider() error = %v", err)
	}

	var executed atomic.Int32
	provider.BindRuntime(AgentRuntimeBinding{
		ExecuteTool: func(ctx context.Context, name string, args map[string]any, meta ToolCallContext) ToolExecutionResult {
			executed.Add(1)
			if name != "echo_tool" {
				t.Fatalf("unexpected tool name %q", name)
			}
			if meta.Channel != "cli" || meta.SessionKey != "session-1" {
				t.Fatalf("unexpected tool context: %+v", meta)
			}
			if args["value"] != "hi" {
				t.Fatalf("unexpected args: %+v", args)
			}
			return ToolExecutionResult{Content: "echo:hi"}
		},
	})

	response, err := provider.Chat(
		context.Background(),
		[]Message{
			{Role: "system", Content: "You are PicoClaw."},
			{Role: "user", Content: "Say hi using the tool."},
		},
		[]ToolDefinition{
			{
				Type: "function",
				Function: ToolFunctionDefinition{
					Name:        "echo_tool",
					Description: "Echo the provided value.",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"value": map[string]any{"type": "string"},
						},
						"required": []string{"value"},
					},
				},
			},
		},
		"lmstudio/local-model",
		map[string]any{
			"max_tokens":                 256,
			rlmOptionKeyChannel:          "cli",
			rlmOptionKeyChatID:           "chat-1",
			rlmOptionKeySessionKey:       "session-1",
			rlmOptionKeyMessageID:        "msg-1",
			rlmOptionKeyAgentID:          "main",
			rlmOptionKeyReplyToMessageID: "",
		},
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if response.Content != "echo:hi" {
		t.Fatalf("response.Content = %q, want %q", response.Content, "echo:hi")
	}
	if response.Usage == nil || response.Usage.TotalTokens != 18 {
		t.Fatalf("unexpected usage: %+v", response.Usage)
	}
	if executed.Load() != 1 {
		t.Fatalf("executed = %d, want 1", executed.Load())
	}
	if callCount.Load() != 1 {
		t.Fatalf("callCount = %d, want 1", callCount.Load())
	}
}

func TestRLMProviderChatErrorsWithoutRuntimeBinding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "```repl\nanswer := echo_tool(map[string]any{\"value\":\"hi\"})\n```\nFINAL_VAR(answer)",
					},
					"finish_reason": "stop",
				},
			},
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider, err := NewRLMProvider(&config.ModelConfig{
		ModelName: "local-rlm",
		Model:     "rlm/lmstudio/local-model",
		APIBase:   server.URL,
	})
	if err != nil {
		t.Fatalf("NewRLMProvider() error = %v", err)
	}

	response, err := provider.Chat(
		context.Background(),
		[]Message{{Role: "user", Content: "Call the tool."}},
		[]ToolDefinition{
			{
				Type: "function",
				Function: ToolFunctionDefinition{
					Name:        "echo_tool",
					Description: "Echo the provided value.",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
		"lmstudio/local-model",
		nil,
	)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if !strings.Contains(response.Content, "tool runtime is not bound") {
		t.Fatalf("expected unbound runtime error in response, got %q", response.Content)
	}
}

func TestRLMProviderBuildEngineConfigInjectsPicoClawClientFactory(t *testing.T) {
	var observedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		observedModel, _ = requestBody["model"].(string)
		response := map[string]any{
			"choices": []map[string]any{{
				"message":       map[string]any{"content": "embedded-response"},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     13,
				"completion_tokens": 5,
				"total_tokens":      18,
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider, err := NewRLMProvider(&config.ModelConfig{
		ModelName: "local-rlm",
		Model:     "rlm/lmstudio/local-model",
		APIBase:   server.URL,
	})
	if err != nil {
		t.Fatalf("NewRLMProvider() error = %v", err)
	}

	engineCfg, _, resolvedModel, err := provider.buildEngineConfig(
		nil,
		nil,
		func() ToolCallContext { return ToolCallContext{} },
		func() context.Context { return context.Background() },
		"lmstudio/local-model",
		false,
	)
	if err != nil {
		t.Fatalf("buildEngineConfig() error = %v", err)
	}
	if engineCfg.ClientFactory == nil {
		t.Fatal("expected engine config to inject a ClientFactory")
	}

	client, err := engineCfg.ClientFactory(engineCfg.Backend, engineCfg.BackendKwargs)
	if err != nil {
		t.Fatalf("ClientFactory() error = %v", err)
	}
	content, err := client.Completion(context.Background(), "hello from PicoClaw", "")
	if err != nil {
		t.Fatalf("Completion() error = %v", err)
	}
	if content != "embedded-response" {
		t.Fatalf("expected embedded-response, got %q", content)
	}
	if resolvedModel != "local-model" {
		t.Fatalf("expected resolvedModel local-model, got %q", resolvedModel)
	}
	if observedModel != "local-model" {
		t.Fatalf("expected observed model local-model, got %q", observedModel)
	}
	usage := client.GetLastUsage()
	if usage.TotalCalls != 1 || usage.TotalInputTokens != 13 || usage.TotalOutputTokens != 5 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
}

