package recursion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// RecursionHook integrates loop detection and compaction with PicoClaw's
// hook system. Mount it as a ToolInterceptor to get automatic loop detection
// on every tool call/result cycle.
//
// Usage:
//
//	hook := recursion.NewRecursionHook(cfg)
//	hookManager.Mount(agent.NamedHook("recursion", hook))
type RecursionHook struct {
	cfg RecursionConfig
}

// Compile-time interface checks.
var (
	_ agent.ToolInterceptor = (*RecursionHook)(nil)
	_ agent.LLMInterceptor  = (*RecursionHook)(nil)
)

func NewRecursionHook(cfg RecursionConfig) *RecursionHook {
	return &RecursionHook{cfg: cfg}
}

// BeforeTool is a no-op passthrough.
func (h *RecursionHook) BeforeTool(
	ctx context.Context,
	call *agent.ToolCallHookRequest,
) (*agent.ToolCallHookRequest, agent.HookDecision, error) {
	return call, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

// AfterTool records the tool call + result in the loop detector and
// aborts the turn if a critical loop is detected.
func (h *RecursionHook) AfterTool(
	ctx context.Context,
	resp *agent.ToolResultHookResponse,
) (*agent.ToolResultHookResponse, agent.HookDecision, error) {
	rs := StateFromContext(ctx)
	if rs == nil || rs.LoopDetector == nil {
		return resp, agent.HookDecision{Action: agent.HookActionContinue}, nil
	}

	// Build a stable representation of the tool call for hashing
	code := resp.Tool
	if len(resp.Arguments) > 0 {
		if b, err := json.Marshal(resp.Arguments); err == nil {
			code += ":" + string(b)
		}
	}

	output := ""
	if resp.Result != nil {
		output = resp.Result.ContentForLLM()
	}

	rs.LoopDetector.Record(code, output)
	lr := rs.LoopDetector.Check()

	if lr.Severity == SeverityCritical {
		rs.Aborted.Store(true)
		// Inject warning into the tool result so the LLM sees it
		if resp.Result != nil {
			resp.Result.ForLLM = fmt.Sprintf(
				"[LOOP DETECTED: %s — %s] %s",
				lr.Detector, lr.Message, resp.Result.ForLLM,
			)
		}
		return resp, agent.HookDecision{
			Action: agent.HookActionAbortTurn,
			Reason: fmt.Sprintf("loop detected: %s (%d occurrences)", lr.Detector, lr.Count),
		}, nil
	}

	if lr.Severity == SeverityWarning && resp.Result != nil {
		resp.Result.ForLLM = fmt.Sprintf(
			"[WARNING: possible loop — %s] %s",
			lr.Message, resp.Result.ForLLM,
		)
	}

	return resp, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

// BeforeLLM checks token budget and injects compaction if needed.
func (h *RecursionHook) BeforeLLM(
	ctx context.Context,
	req *agent.LLMHookRequest,
) (*agent.LLMHookRequest, agent.HookDecision, error) {
	rs := StateFromContext(ctx)
	if rs == nil {
		return req, agent.HookDecision{Action: agent.HookActionContinue}, nil
	}

	// Check abort
	if rs.Aborted.Load() {
		return req, agent.HookDecision{
			Action: agent.HookActionAbortTurn,
			Reason: "recursion aborted",
		}, nil
	}

	// Check iteration limit
	if h.cfg.MaxIterations > 0 && rs.Iteration >= h.cfg.MaxIterations {
		return req, agent.HookDecision{
			Action: agent.HookActionAbortTurn,
			Reason: fmt.Sprintf("max recursion iterations (%d) reached", h.cfg.MaxIterations),
		}, nil
	}

	rs.Iteration++
	return req, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

// AfterLLM tracks token usage from the response.
func (h *RecursionHook) AfterLLM(
	ctx context.Context,
	resp *agent.LLMHookResponse,
) (*agent.LLMHookResponse, agent.HookDecision, error) {
	rs := StateFromContext(ctx)
	if rs == nil || resp.Response == nil || resp.Response.Usage == nil {
		return resp, agent.HookDecision{Action: agent.HookActionContinue}, nil
	}

	rs.TokensUsed.Add(int64(resp.Response.Usage.TotalTokens))

	// Check token budget
	if rs.TokenBudget > 0 && rs.TokensUsed.Load() > rs.TokenBudget {
		return resp, agent.HookDecision{
			Action: agent.HookActionAbortTurn,
			Reason: fmt.Sprintf("token budget exhausted (%d/%d)",
				rs.TokensUsed.Load(), rs.TokenBudget),
		}, nil
	}

	return resp, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

// MCTSTool exposes MCTS branching as a PicoClaw tool.
// The LLM can call "mcts_explore" to spawn parallel exploration branches.
type MCTSTool struct {
	spawner SubTurnSpawner
	model   string
	cfg     MCTSConfig
}

var _ tools.Tool = (*MCTSTool)(nil)

func NewMCTSTool(spawner SubTurnSpawner, model string, cfg MCTSConfig) *MCTSTool {
	return &MCTSTool{spawner: spawner, model: model, cfg: cfg}
}

func (t *MCTSTool) Name() string { return "mcts_explore" }

func (t *MCTSTool) Description() string {
	return "Explore multiple solution paths in parallel using MCTS-style branching. " +
		"Strong branches are refined across multiple rounds and the best result is returned."
}

func (t *MCTSTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "The task to explore from multiple angles",
			},
			"branches": map[string]any{
				"type":        "integer",
				"description": "Number of parallel branches (default 3, max 8)",
			},
			"depth": map[string]any{
				"type":        "integer",
				"description": "Number of refinement rounds (default from config, max 6)",
			},
		},
		"required": []string{"task"},
	}
}

func (t *MCTSTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	task, _ := args["task"].(string)
	if task == "" {
		return tools.ErrorResult("mcts_explore: 'task' is required")
	}

	cfg := t.cfg
	cfg.Model = t.model
	if b, ok := args["branches"].(float64); ok && b >= 1 && b <= 8 {
		cfg.Branches = int(b)
	}
	if d, ok := args["depth"].(float64); ok && d >= 1 && d <= 6 {
		cfg.Depth = int(d)
	}

	winner, all, err := RunMCTS(ctx, t.spawner, task, cfg)
	if err != nil {
		return tools.ErrorResult(fmt.Sprintf("mcts_explore failed: %v", err))
	}

	// Format results for LLM
	winnerScore := fmt.Sprintf("%.1f", winner.Score)
	if winner.IsError {
		winnerScore = "error"
	}

	summary := fmt.Sprintf(
		"MCTS explored %d candidates across %d rounds. Winner: %s (score: %s, iter: %d)\n\n",
		len(all), cfg.Depth, winner.ID, winnerScore, winner.Iterations,
	)

	displayCount := len(all)
	if displayCount > 6 {
		displayCount = 6
	}

	for _, br := range all[:displayCount] {
		scoreLabel := fmt.Sprintf("%.1f", br.Score)
		if br.IsError {
			scoreLabel = "error"
		}

		summary += fmt.Sprintf("--- %s (score: %s, iter: %d, %v) ---\n%s\n\n",
			br.ID,
			scoreLabel,
			br.Iterations,
			br.Duration.Round(time.Millisecond),
			truncate(br.Content, 500),
		)
	}

	if len(all) > displayCount {
		summary += fmt.Sprintf("... %d additional candidates omitted ...\n\n", len(all)-displayCount)
	}

	summary += fmt.Sprintf("=== BEST RESULT ===\n%s", winner.Content)

	return tools.NewToolResult(summary)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
