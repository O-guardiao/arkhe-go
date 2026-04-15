package recursion

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/tools"
)

// BranchResult holds the outcome of a single MCTS exploration branch.
type BranchResult struct {
	ID         string
	Content    string
	Score      float64
	Iterations int
	IsError    bool
	Duration   time.Duration
}

// MCTSConfig configures parallel branch exploration.
type MCTSConfig struct {
	Branches int           // Number of parallel branches (default 3)
	Depth    int           // Max iterations per branch (default 2)
	Timeout  time.Duration // Per-branch timeout
	Model    string        // LLM model for branches
	Scorer   BranchScorer  // Custom scorer (nil = default)
}

// BranchScorer evaluates a branch result. Higher = better.
type BranchScorer func(result BranchResult) float64

type branchStrategy struct {
	Name        string
	Instruction string
}

var defaultBranchStrategies = []branchStrategy{
	{
		Name:        "direct",
		Instruction: "Push directly toward the strongest complete answer.",
	},
	{
		Name:        "planner",
		Instruction: "Decompose the task into a compact plan and then execute it concretely.",
	},
	{
		Name:        "critic",
		Instruction: "Challenge likely weak spots first, then produce the repaired answer.",
	},
	{
		Name:        "minimal",
		Instruction: "Prefer the smallest safe answer that still fully satisfies the task.",
	},
	{
		Name:        "alternative",
		Instruction: "Try a materially different angle from the obvious approach.",
	},
	{
		Name:        "verify",
		Instruction: "Bias toward edge cases, verification, and failure prevention.",
	},
}

// DefaultScorer implements RLM's scoring heuristic:
// +2 if no error, -2 if error, +1 if has output, +1 if output contains numbers,
// -0.5 if content is very short (<30 chars).
func DefaultScorer(r BranchResult) float64 {
	score := 0.0
	if r.IsError {
		score -= 2.0
	} else {
		score += 2.0
	}
	content := strings.TrimSpace(r.Content)
	if len(content) > 0 {
		score += 1.0
	}
	if hasDigit(content) {
		score += 1.0
	}
	if len(content) < 30 {
		score -= 0.5
	}
	return score
}

// RunMCTS spawns N parallel SubTurns for the same task, scores them,
// and returns the winner. Later rounds iteratively refine the strongest
// candidates instead of re-running identical prompts.
func RunMCTS(
	ctx context.Context,
	spawner SubTurnSpawner,
	task string,
	cfg MCTSConfig,
) (*BranchResult, []BranchResult, error) {
	if cfg.Branches <= 0 {
		cfg.Branches = 3
	}
	if cfg.Depth <= 0 {
		cfg.Depth = 2
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	if cfg.Model == "" {
		return nil, nil, fmt.Errorf("mcts: model is required")
	}
	scorer := cfg.Scorer
	if scorer == nil {
		scorer = DefaultScorer
	}

	allResults := make([]BranchResult, 0, cfg.Branches*cfg.Depth)
	var survivors []BranchResult

	for depth := 1; depth <= cfg.Depth; depth++ {
		roundResults := runMCTSRound(ctx, spawner, task, survivors, depth, cfg, scorer)
		if len(roundResults) == 0 {
			break
		}
		allResults = append(allResults, roundResults...)
		survivors = topResults(roundResults, beamWidth(cfg.Branches))
	}

	if len(allResults) == 0 {
		return nil, nil, fmt.Errorf("mcts: no branches were explored")
	}

	sortBranchResults(allResults)
	winner := allResults[0]
	return &winner, allResults, nil
}

func runMCTSRound(
	ctx context.Context,
	spawner SubTurnSpawner,
	task string,
	parents []BranchResult,
	depth int,
	cfg MCTSConfig,
	scorer BranchScorer,
) []BranchResult {
	type indexed struct {
		idx    int
		result BranchResult
	}

	results := make([]BranchResult, cfg.Branches)
	var wg sync.WaitGroup
	ch := make(chan indexed, cfg.Branches)

	for i := 0; i < cfg.Branches; i++ {
		strategy := branchStrategyFor(depth, i)
		var parent *BranchResult
		if len(parents) > 0 {
			parentCopy := parents[i%len(parents)]
			parent = &parentCopy
		}

		wg.Add(1)
		go func(idx int, strategy branchStrategy, parent *BranchResult) {
			defer wg.Done()
			br := exploreBranch(ctx, spawner, task, idx, depth, cfg, strategy, parent)
			if br.Score != -math.MaxFloat64 {
				br.Score = scorer(br)
			}
			ch <- indexed{idx: idx, result: br}
		}(i, strategy, parent)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for ir := range ch {
		results[ir.idx] = ir.result
	}

	sortBranchResults(results)
	return results
}

func exploreBranch(
	ctx context.Context,
	spawner SubTurnSpawner,
	task string,
	idx int,
	depth int,
	cfg MCTSConfig,
	strategy branchStrategy,
	parent *BranchResult,
) BranchResult {
	branchID := fmt.Sprintf("mcts-r%d-b%d-%s", depth, idx, strategy.Name)
	start := time.Now()

	stCfg := tools.SubTurnConfig{
		Model:              cfg.Model,
		SystemPrompt:       buildBranchTaskPrompt(task, parent),
		ActualSystemPrompt: buildBranchSystemPrompt(strategy, depth, cfg.Depth),
		Timeout:            cfg.Timeout,
		Async:              false,
	}

	result, err := spawner.SpawnSubTurn(ctx, stCfg)
	totalDuration := time.Since(start)
	if parent != nil {
		totalDuration += parent.Duration
	}

	if err != nil {
		return BranchResult{
			ID:         branchID,
			Content:    fmt.Sprintf("error: %v", err),
			Iterations: depth,
			IsError:    true,
			Duration:   totalDuration,
			Score:      -math.MaxFloat64,
		}
	}

	content := ""
	isErr := false
	if result != nil {
		content = result.ContentForLLM()
		isErr = result.IsError
	}

	score := 0.0
	if isErr {
		score = -math.MaxFloat64
	}

	return BranchResult{
		ID:         branchID,
		Content:    content,
		Score:      score,
		Iterations: depth,
		IsError:    isErr,
		Duration:   totalDuration,
	}
}

func branchStrategyFor(depth, idx int) branchStrategy {
	return defaultBranchStrategies[(depth-1+idx)%len(defaultBranchStrategies)]
}

func buildBranchSystemPrompt(strategy branchStrategy, depth, maxDepth int) string {
	return fmt.Sprintf(
		"You are one branch in an MCTS-style search over LLM subturns. Round %d of %d. Strategy: %s. %s Return only the candidate answer, not commentary about the search process.",
		depth,
		maxDepth,
		strategy.Name,
		strategy.Instruction,
	)
}

func buildBranchTaskPrompt(task string, parent *BranchResult) string {
	task = strings.TrimSpace(task)
	if parent == nil {
		return task
	}

	parentContent := truncateForPrompt(strings.TrimSpace(parent.Content), 2000)
	if parentContent == "" {
		return fmt.Sprintf(
			"Original task:\n%s\n\nPrevious branch produced no useful output. Produce a stronger complete answer.",
			task,
		)
	}

	if parent.IsError {
		return fmt.Sprintf(
			"Original task:\n%s\n\nPrevious attempt failed with:\n%s\n\nProduce a fresh, stronger candidate answer.",
			task,
			parentContent,
		)
	}

	return fmt.Sprintf(
		"Original task:\n%s\n\nPrevious candidate to refine:\n%s\n\nProduce a stronger candidate. Keep what is correct, remove weak parts, and make the result more concrete and verifiable. Do not mention the prior branch in the final answer.",
		task,
		parentContent,
	)
}

func beamWidth(branches int) int {
	if branches <= 1 {
		return 1
	}
	return (branches + 1) / 2
}

func topResults(results []BranchResult, limit int) []BranchResult {
	if len(results) == 0 || limit <= 0 {
		return nil
	}
	if limit > len(results) {
		limit = len(results)
	}
	top := make([]BranchResult, limit)
	copy(top, results[:limit])
	return top
}

func sortBranchResults(results []BranchResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Iterations != results[j].Iterations {
			return results[i].Iterations > results[j].Iterations
		}
		return results[i].Duration < results[j].Duration
	})
}

func truncateForPrompt(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func hasDigit(s string) bool {
	for _, c := range s {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}
