package utils_test

import (
	"testing"

	"github.com/O-guardiao/arkhe-go/rlm-go/environments"
	"github.com/O-guardiao/arkhe-go/rlm-go/types"
	"github.com/O-guardiao/arkhe-go/rlm-go/utils"
)

func TestFindCodeBlocks(t *testing.T) {
	text := "Before\n```repl\nx := 1 + 2\nprint(x)\n```\nAfter"
	blocks := utils.FindCodeBlocks(text)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0] != "x := 1 + 2\nprint(x)" {
		t.Fatalf("unexpected block content: %q", blocks[0])
	}
}

func TestFindFinalAnswer(t *testing.T) {
	answer, ok := utils.FindFinalAnswer("FINAL(42)", nil)
	if !ok || answer != "42" {
		t.Fatalf("expected FINAL(42), got ok=%v answer=%q", ok, answer)
	}
}

func TestFindFinalVarWithResolver(t *testing.T) {
	repl, err := environments.NewLocalREPL(environments.Config{})
	if err != nil {
		t.Fatalf("new repl: %v", err)
	}
	defer repl.Cleanup()

	result := repl.ExecuteCode(`answer := 42`)
	if result.Stderr != "" {
		t.Fatalf("unexpected stderr: %s", result.Stderr)
	}

	answer, ok := utils.FindFinalAnswer("FINAL_VAR(answer)", repl)
	if !ok || answer != "42" {
		t.Fatalf("expected final var 42, got ok=%v answer=%q", ok, answer)
	}
}

func TestFormatIteration(t *testing.T) {
	iteration := types.RLMIteration{
		Response: "thinking",
		CodeBlocks: []types.CodeBlock{
			{
				Code: "x := 1 + 2\nprint(x)",
				Result: types.REPLResult{
					Stdout: "3\n",
					Locals: map[string]any{"x": 3},
				},
			},
		},
	}
	messages := utils.FormatIteration(iteration, 20000)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}
