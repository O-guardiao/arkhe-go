package recursion

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/tools"
)

type recordingSpawner struct {
	mu    sync.Mutex
	calls []tools.SubTurnConfig
}

func (s *recordingSpawner) SpawnSubTurn(ctx context.Context, cfg tools.SubTurnConfig) (*tools.ToolResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, cfg)
	s.mu.Unlock()

	switch {
	case strings.Contains(cfg.SystemPrompt, "Previous candidate") && strings.Contains(cfg.SystemPrompt, "planner initial 123"):
		return tools.NewToolResult("planner refined 123 456 with verification and concrete steps"), nil
	case strings.Contains(cfg.SystemPrompt, "Previous candidate") && strings.Contains(cfg.SystemPrompt, "direct initial candidate"):
		return tools.NewToolResult("direct refined candidate"), nil
	case strings.Contains(cfg.ActualSystemPrompt, "Strategy: planner"):
		return tools.NewToolResult("planner initial 123"), nil
	case strings.Contains(cfg.ActualSystemPrompt, "Strategy: direct"):
		return tools.NewToolResult("direct initial candidate with concrete detail"), nil
	case strings.Contains(cfg.ActualSystemPrompt, "Strategy: critic"):
		return tools.NewToolResult("critic short"), nil
	case strings.Contains(cfg.ActualSystemPrompt, "Strategy: minimal"):
		return tools.NewToolResult("minimal initial answer"), nil
	default:
		return tools.NewToolResult("fallback initial answer"), nil
	}
}

func (s *recordingSpawner) Calls() []tools.SubTurnConfig {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]tools.SubTurnConfig, len(s.calls))
	copy(out, s.calls)
	return out
}

func TestRunMCTS_UsesDepthAndRefinement(t *testing.T) {
	spawner := &recordingSpawner{}

	winner, all, err := RunMCTS(context.Background(), spawner, "Solve the task completely.", MCTSConfig{
		Branches: 3,
		Depth:    2,
		Timeout:  time.Second,
		Model:    "test-model",
	})
	if err != nil {
		t.Fatalf("RunMCTS returned error: %v", err)
	}
	if winner == nil {
		t.Fatal("winner should not be nil")
	}
	if len(all) != 6 {
		t.Fatalf("expected 6 total candidates, got %d", len(all))
	}
	if winner.Iterations != 2 {
		t.Fatalf("expected winning candidate to come from refinement round, got iteration %d", winner.Iterations)
	}
	if !strings.Contains(winner.Content, "planner refined") {
		t.Fatalf("expected refined planner result to win, got %q", winner.Content)
	}

	var sawRefinement bool
	for _, call := range spawner.Calls() {
		if strings.Contains(call.SystemPrompt, "Previous candidate") {
			sawRefinement = true
			break
		}
	}
	if !sawRefinement {
		t.Fatal("expected at least one refinement prompt in later rounds")
	}
}

func TestRunMCTS_DiversifiesInitialStrategies(t *testing.T) {
	spawner := &recordingSpawner{}

	_, _, err := RunMCTS(context.Background(), spawner, "Solve the task completely.", MCTSConfig{
		Branches: 4,
		Depth:    1,
		Timeout:  time.Second,
		Model:    "test-model",
	})
	if err != nil {
		t.Fatalf("RunMCTS returned error: %v", err)
	}

	seenStrategies := map[string]bool{}
	for _, call := range spawner.Calls() {
		for _, strategy := range []string{"direct", "planner", "critic", "minimal", "alternative", "verify"} {
			if strings.Contains(call.ActualSystemPrompt, "Strategy: "+strategy) {
				seenStrategies[strategy] = true
			}
		}
	}

	if len(seenStrategies) < 4 {
		t.Fatalf("expected 4 distinct initial strategies, got %d (%v)", len(seenStrategies), seenStrategies)
	}
}

func TestRunMCTS_RequiresModel(t *testing.T) {
	_, _, err := RunMCTS(context.Background(), &recordingSpawner{}, "task", MCTSConfig{Branches: 2})
	if err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("expected model validation error, got %v", err)
	}
}

