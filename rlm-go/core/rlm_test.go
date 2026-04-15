package core_test

import (
	"testing"

	"github.com/alexzhang13/rlm-go/core"
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
