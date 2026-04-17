package environments_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/O-guardiao/arkhe-go/rlm-go/environments"
	"github.com/O-guardiao/arkhe-go/rlm-go/types"
)

func TestLocalREPLSimpleExecution(t *testing.T) {
	repl, err := environments.NewLocalREPL(environments.Config{})
	if err != nil {
		t.Fatalf("new repl: %v", err)
	}
	defer repl.Cleanup()

	result := repl.ExecuteCode(`x := 1 + 2`)
	if result.Stderr != "" {
		t.Fatalf("unexpected stderr: %s", result.Stderr)
	}
	if got := result.Locals["x"]; got != 3 {
		t.Fatalf("expected x=3, got %#v", got)
	}
}

func TestLocalREPLPrintAndPersistence(t *testing.T) {
	repl, err := environments.NewLocalREPL(environments.Config{})
	if err != nil {
		t.Fatalf("new repl: %v", err)
	}
	defer repl.Cleanup()

	first := repl.ExecuteCode(`greeting := "hello"`)
	if first.Stderr != "" {
		t.Fatalf("unexpected stderr: %s", first.Stderr)
	}
	second := repl.ExecuteCode(`print(greeting)`)
	if second.Stderr != "" {
		t.Fatalf("unexpected stderr: %s", second.Stderr)
	}
	if second.Stdout != "hello\n" {
		t.Fatalf("expected hello stdout, got %q", second.Stdout)
	}
}

func TestLocalREPLContextAndRestoration(t *testing.T) {
	repl, err := environments.NewLocalREPL(environments.Config{ContextPayload: "original context"})
	if err != nil {
		t.Fatalf("new repl: %v", err)
	}
	defer repl.Cleanup()

	if got := repl.ExecuteCode(`print(context)`).Stdout; got != "original context\n" {
		t.Fatalf("unexpected initial context: %q", got)
	}
	repl.ExecuteCode(`context = "hijacked"`)
	if got := repl.ExecuteCode(`print(context)`).Stdout; got != "original context\n" {
		t.Fatalf("context was not restored, got %q", got)
	}
}

func TestLocalREPLFinalVar(t *testing.T) {
	repl, err := environments.NewLocalREPL(environments.Config{})
	if err != nil {
		t.Fatalf("new repl: %v", err)
	}
	defer repl.Cleanup()

	repl.ExecuteCode(`answer := 42`)
	result := repl.ExecuteCode(`result := FINAL_VAR("answer")`)
	if result.Stderr != "" {
		t.Fatalf("unexpected stderr: %s", result.Stderr)
	}
	if got := result.Locals["result"]; got != "42" {
		t.Fatalf("expected result=42, got %#v", got)
	}
}

func TestLocalREPLRecursiveQueryBatch(t *testing.T) {
	subcallFn := func(prompt string, model string) (types.RLMChatCompletion, error) {
		return types.RLMChatCompletion{
			RootModel:     "mock",
			Prompt:        prompt,
			Response:      "resp-" + prompt,
			UsageSummary:  types.UsageSummary{ModelUsageSummaries: map[string]types.ModelUsageSummary{}},
			ExecutionTime: 0.01,
		}, nil
	}
	repl, err := environments.NewLocalREPL(environments.Config{
		SubcallFn:             subcallFn,
		MaxConcurrentSubcalls: 4,
	})
	if err != nil {
		t.Fatalf("new repl: %v", err)
	}
	defer repl.Cleanup()

	result := repl.ExecuteCode(`answers := rlm_query_batched([]string{"a", "b", "c"}, "")`)
	if result.Stderr != "" {
		t.Fatalf("unexpected stderr: %s", result.Stderr)
	}
	got, ok := result.Locals["answers"].([]any)
	if !ok {
		t.Fatalf("expected []string answers, got %#v", result.Locals["answers"])
	}
	expected := []string{"resp-a", "resp-b", "resp-c"}
	for i, item := range expected {
		if got[i] != item {
			t.Fatalf("expected %q at %d, got %#v", item, i, got[i])
		}
	}
}

func TestLocalREPLUsesConfiguredWorkingDirWithoutContextFiles(t *testing.T) {
	workingDir := t.TempDir()
	repl, err := environments.NewLocalREPL(environments.Config{
		ContextPayload: "original context",
		WorkingDir:     workingDir,
	})
	if err != nil {
		t.Fatalf("new repl: %v", err)
	}
	defer repl.Cleanup()

	result := repl.ExecuteCode(`import "os"
func currentDir() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}
var cwd = currentDir()`)
	if result.Stderr != "" {
		t.Fatalf("unexpected stderr: %s", result.Stderr)
	}
	if got := result.Locals["cwd"]; got != workingDir {
		t.Fatalf("expected cwd=%q, got %#v", workingDir, got)
	}

	for _, path := range []string{
		filepath.Join(workingDir, "context_0.txt"),
		filepath.Join(workingDir, "context_0.json"),
	} {
		_, err := os.Stat(path)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected no context artifact at %q, got err=%v", path, err)
		}
	}
}
