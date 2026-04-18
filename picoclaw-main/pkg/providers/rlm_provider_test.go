package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// TestRLMSessionStateMetaContextRaceGuard documents and guards the invariant
// broken by an earlier version of chatWithSession: per-session meta/ctx MUST
// only be mutated while holding state.mu. Concurrent Chat() calls for the same
// sessionKey that race setMeta/setContext outside the critical section cause
// cross-contamination of channel/chat_id routing — a tool executing on behalf
// of caller A could observe caller B's meta and publish bus messages to the
// wrong user.
//
// This test proves the locking discipline: a writer holding the lock is never
// observed with a foreign writer's meta, even when a second writer is racing
// to enter the critical section.
func TestRLMSessionStateMetaContextRaceGuard(t *testing.T) {
	state := &rlmSessionState{}

	const iters = 200
	var wg sync.WaitGroup
	wg.Add(2)

	// Writer A: repeatedly claims the lock, installs its meta, verifies no
	// foreign meta leaked into its critical section, releases.
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			state.mu.Lock()
			state.setMeta(ToolCallContext{SessionKey: "A", Channel: "chanA"})
			// Simulate engine.Completion doing work with multiple tool
			// invocations reading state.meta() lazily.
			for j := 0; j < 5; j++ {
				if got := state.meta(); got.Channel != "chanA" {
					state.mu.Unlock()
					t.Errorf("A observed foreign meta mid-turn: %+v", got)
					return
				}
				time.Sleep(50 * time.Microsecond)
			}
			state.mu.Unlock()
		}
	}()

	// Writer B: same discipline with a different identity.
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			state.mu.Lock()
			state.setMeta(ToolCallContext{SessionKey: "B", Channel: "chanB"})
			for j := 0; j < 5; j++ {
				if got := state.meta(); got.Channel != "chanB" {
					state.mu.Unlock()
					t.Errorf("B observed foreign meta mid-turn: %+v", got)
					return
				}
				time.Sleep(50 * time.Microsecond)
			}
			state.mu.Unlock()
		}
	}()

	wg.Wait()
}

// TestRLMProviderChatWithSessionIsolatesMetaUnderConcurrency exercises the
// full chatWithSession path under contention: two concurrent Chat() calls
// against the SAME sessionKey must each observe their own meta inside the
// bound tool. Before the fix, setMeta/setContext ran before state.mu.Lock(),
// so the second caller could overwrite the first caller's meta while the
// first was still inside engine.Completion, leaking channel/chat_id across
// tenants.
func TestRLMProviderChatWithSessionIsolatesMetaUnderConcurrency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": "```repl\nanswer := echo_tool(map[string]any{\"value\":\"hi\"})\n```\nFINAL_VAR(answer)",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2,
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

	var mu sync.Mutex
	observed := map[string][]string{} // expected channel -> observed channels
	provider.BindRuntime(AgentRuntimeBinding{
		ExecuteTool: func(ctx context.Context, name string, args map[string]any, meta ToolCallContext) ToolExecutionResult {
			// Hold briefly so both goroutines overlap on the sessionKey.
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			observed[meta.ChatID] = append(observed[meta.ChatID], meta.Channel)
			mu.Unlock()
			return ToolExecutionResult{Content: "ok"}
		},
	})

	sessionKey := "shared-session"
	makeOpts := func(channel, chatID string) map[string]any {
		return map[string]any{
			rlmOptionKeyChannel:    channel,
			rlmOptionKeyChatID:     chatID,
			rlmOptionKeySessionKey: sessionKey,
			rlmOptionKeyAgentID:    "main",
			"max_tokens":           32,
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, err := provider.Chat(context.Background(),
				[]Message{{Role: "user", Content: "hi"}},
				[]ToolDefinition{{
					Type: "function",
					Function: ToolFunctionDefinition{
						Name:        "echo_tool",
						Description: "echo",
						Parameters:  map[string]any{"type": "object"},
					},
				}},
				"lmstudio/local-model", makeOpts("chanA", "chatA"))
			if err != nil {
				t.Errorf("A Chat error: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_, err := provider.Chat(context.Background(),
				[]Message{{Role: "user", Content: "hi"}},
				[]ToolDefinition{{
					Type: "function",
					Function: ToolFunctionDefinition{
						Name:        "echo_tool",
						Description: "echo",
						Parameters:  map[string]any{"type": "object"},
					},
				}},
				"lmstudio/local-model", makeOpts("chanB", "chatB"))
			if err != nil {
				t.Errorf("B Chat error: %v", err)
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	for _, ch := range observed["chatA"] {
		if ch != "chanA" {
			t.Fatalf("chatA observed foreign channel %q — meta cross-contamination", ch)
		}
	}
	for _, ch := range observed["chatB"] {
		if ch != "chanB" {
			t.Fatalf("chatB observed foreign channel %q — meta cross-contamination", ch)
		}
	}
	if len(observed["chatA"]) == 0 || len(observed["chatB"]) == 0 {
		t.Fatalf("expected both tenants to record tool calls, got %+v", observed)
	}
}
