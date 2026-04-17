package core_test

import (
	"testing"

	"github.com/O-guardiao/arkhe-go/rlm-go/clients"
	"github.com/O-guardiao/arkhe-go/rlm-go/core"
)

func TestRLMBasicFinalAnswer(t *testing.T) {
	rlm, err := core.New(core.Config{
		Backend:       "mock",
		BackendKwargs: map[string]any{"model_name": "mock-model", "responses": []string{"FINAL(42)"}},
		MaxDepth:      1,
	})
	if err != nil {
		t.Fatalf("new rlm: %v", err)
	}
	result, err := rlm.Completion("What is the answer?", "")
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	if result.Response != "42" {
		t.Fatalf("expected 42, got %q", result.Response)
	}
}

func TestRLMFinalVarAnswer(t *testing.T) {
	rlm, err := core.New(core.Config{
		Backend: "mock",
		BackendKwargs: map[string]any{
			"model_name": "mock-model",
			"responses": []string{
				"```repl\nx := 21 * 2\n```\n",
				"FINAL_VAR(x)",
			},
		},
		MaxDepth: 1,
	})
	if err != nil {
		t.Fatalf("new rlm: %v", err)
	}
	result, err := rlm.Completion("Compute 42", "")
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	if result.Response != "42" {
		t.Fatalf("expected 42, got %q", result.Response)
	}
}

func TestRLMFallbackUsesInjectedClientFactory(t *testing.T) {
	var calls int
	var seenBackend string
	var seenModel string

	rlm, err := core.New(core.Config{
		Backend:       "ignored-backend",
		BackendKwargs: map[string]any{"model_name": "factory-model"},
		Depth:         1,
		MaxDepth:      1,
		ClientFactory: func(backend string, backendKwargs map[string]any) (clients.Client, error) {
			calls++
			seenBackend = backend
			seenModel = backendKwargs["model_name"].(string)
			return clients.NewMockClient("factory-model", []string{"factory-response"}, nil), nil
		},
	})
	if err != nil {
		t.Fatalf("new rlm: %v", err)
	}

	result, err := rlm.Completion("What is the answer?", "")
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	if result.Response != "factory-response" {
		t.Fatalf("expected factory-response, got %q", result.Response)
	}
	if calls != 1 {
		t.Fatalf("expected 1 factory call, got %d", calls)
	}
	if seenBackend != "ignored-backend" {
		t.Fatalf("expected backend ignored-backend, got %q", seenBackend)
	}
	if seenModel != "factory-model" {
		t.Fatalf("expected model factory-model, got %q", seenModel)
	}
}

func TestRLMNestedSubcallUsesInjectedClientFactoryAtMaxDepth(t *testing.T) {
	responses := []string{
		"```repl\nmid := rlm_query(\"middle\", \"\")\n```\nFINAL_VAR(mid)",
		"```repl\nleaf := rlm_query(\"leaf\", \"\")\n```\nFINAL_VAR(leaf)",
		"leaf-result",
	}
	var calls int

	rlm, err := core.New(core.Config{
		Backend:       "ignored-backend",
		BackendKwargs: map[string]any{"model_name": "factory-model"},
		Environment:   "local",
		MaxDepth:      2,
		ClientFactory: func(backend string, backendKwargs map[string]any) (clients.Client, error) {
			if calls >= len(responses) {
				t.Fatalf("unexpected extra factory call %d", calls+1)
			}
			response := responses[calls]
			calls++
			modelName, _ := backendKwargs["model_name"].(string)
			return clients.NewMockClient(modelName, []string{response}, nil), nil
		},
	})
	if err != nil {
		t.Fatalf("new rlm: %v", err)
	}

	result, err := rlm.Completion("Start recursion", "")
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	if result.Response != "leaf-result" {
		t.Fatalf("expected leaf-result, got %q", result.Response)
	}
	if calls != len(responses) {
		t.Fatalf("expected %d factory calls, got %d", len(responses), calls)
	}
	if err := rlm.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
