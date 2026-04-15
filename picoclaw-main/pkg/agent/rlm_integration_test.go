package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestAgentLoop_RLMProviderUsesLocalTools(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("hello from picoclaw"), 0o644); err != nil {
		t.Fatalf("WriteFile(note.txt): %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "```repl\nanswer := read_file(map[string]any{\"path\":\"note.txt\"})\n```\nFINAL_VAR(answer)",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     12,
				"completion_tokens": 8,
				"total_tokens":      20,
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("Encode(response): %v", err)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = workspace
	cfg.Agents.Defaults.ModelName = "local-rlm"
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "local-rlm",
			Model:     "rlm/lmstudio/local-model",
			APIBase:   server.URL,
			Enabled:   true,
		},
	}

	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider(): %v", err)
	}

	msgBus := bus.NewMessageBus()
	defer msgBus.Close()

	loop := NewAgentLoop(cfg, msgBus, provider)
	defer loop.Close()

	response, err := loop.ProcessDirect(context.Background(), "Leia o arquivo local.", "cli:test")
	if err != nil {
		t.Fatalf("ProcessDirect(): %v", err)
	}
	if !strings.Contains(response, "hello from picoclaw") {
		t.Fatalf("response = %q, want content to contain %q", response, "hello from picoclaw")
	}
	if !strings.Contains(response, "[file: note.txt") {
		t.Fatalf("response = %q, want read_file header", response)
	}
}

func TestAgentLoop_RLMProviderPersistsSessionState(t *testing.T) {
	workspace := t.TempDir()
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := callCount.Add(1)
		content := ""
		switch idx {
		case 1:
			content = "```repl\ngreeting := \"hello from rlm\"\nstored := \"stored\"\n```\nFINAL_VAR(stored)"
		case 2:
			content = "FINAL_VAR(greeting)"
		default:
			t.Fatalf("unexpected backend call %d", idx)
		}

		response := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": content,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]any{
				"prompt_tokens":     6,
				"completion_tokens": 4,
				"total_tokens":      10,
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("Encode(response): %v", err)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = workspace
	cfg.Agents.Defaults.ModelName = "local-rlm"
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "local-rlm",
			Model:     "rlm/lmstudio/local-model",
			APIBase:   server.URL,
			Enabled:   true,
		},
	}

	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider(): %v", err)
	}

	msgBus := bus.NewMessageBus()
	defer msgBus.Close()

	loop := NewAgentLoop(cfg, msgBus, provider)
	defer loop.Close()

	first, err := loop.ProcessDirect(context.Background(), "Guarde uma variavel local.", "cli:stateful")
	if err != nil {
		t.Fatalf("first ProcessDirect(): %v", err)
	}
	if first != "stored" {
		t.Fatalf("first response = %q, want %q", first, "stored")
	}

	second, err := loop.ProcessDirect(context.Background(), "Recupere a variavel local.", "cli:stateful")
	if err != nil {
		t.Fatalf("second ProcessDirect(): %v", err)
	}
	if second != "hello from rlm" {
		t.Fatalf("second response = %q, want %q", second, "hello from rlm")
	}
	if callCount.Load() != 2 {
		t.Fatalf("callCount = %d, want 2", callCount.Load())
	}
}

func TestAgentLoop_RLMProviderUsesAgentWorkspaceAsWorkingDir(t *testing.T) {
	workspace := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "```repl\nimport \"os\"\nfunc currentDir() string {\n\twd, err := os.Getwd()\n\tif err != nil {\n\t\tpanic(err)\n\t}\n\treturn wd\n}\nvar wd = currentDir()\n```\nFINAL_VAR(wd)",
					},
					"finish_reason": "stop",
				},
			},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("Encode(response): %v", err)
		}
	}))
	defer server.Close()

	cfg := config.DefaultConfig()
	cfg.Agents.Defaults.Workspace = workspace
	cfg.Agents.Defaults.ModelName = "local-rlm"
	cfg.ModelList = []*config.ModelConfig{
		{
			ModelName: "local-rlm",
			Model:     "rlm/lmstudio/local-model",
			APIBase:   server.URL,
			Enabled:   true,
		},
	}

	provider, _, err := providers.CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider(): %v", err)
	}

	msgBus := bus.NewMessageBus()
	defer msgBus.Close()

	loop := NewAgentLoop(cfg, msgBus, provider)
	defer loop.Close()

	response, err := loop.ProcessDirect(context.Background(), "Mostre o diretório atual.", "cli:cwd")
	if err != nil {
		t.Fatalf("ProcessDirect(): %v", err)
	}
	if response != filepath.Clean(workspace) {
		t.Fatalf("response = %q, want %q", response, filepath.Clean(workspace))
	}

	matches, err := filepath.Glob(filepath.Join(workspace, "context_*"))
	if err != nil {
		t.Fatalf("Glob(context_*): %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no context artifacts in workspace, got %v", matches)
	}
}
