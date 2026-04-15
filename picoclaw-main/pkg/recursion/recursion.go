// Package recursion adds RLM-style recursion capabilities to PicoClaw:
// loop detection, MCTS branching, execution supervision, and semantic memory.
//
// It integrates with PicoClaw's existing SubTurn, tool loop, and hook systems
// rather than replacing them. The recursion package is an overlay — not a fork.
package recursion

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// RecursionConfig controls how the recursion engine behaves.
type RecursionConfig struct {
	// Loop detection
	LoopDetector LoopDetectorConfig

	// MCTS branching
	MCTSBranches   int           // Parallel branches to explore (0 = disabled)
	MCTSDepth      int           // Max iterations per branch
	MCTSTimeout    time.Duration // Timeout per branch

	// Supervisor
	MaxExecutionTime time.Duration // Hard timeout for the whole recursion
	MaxIterations    int           // Iteration cap (inherited from tool loop if 0)

	// Compaction
	CompactionThreshold float64 // 0.85 = compact when 85% of context used
	CompactionKeepLast  int     // Messages to preserve after compaction
}

// DefaultConfig returns sensible defaults matching RLM's production values.
func DefaultConfig() RecursionConfig {
	return RecursionConfig{
		LoopDetector:        DefaultLoopDetectorConfig(),
		MCTSBranches:        3,
		MCTSDepth:           2,
		MCTSTimeout:         90 * time.Second,
		MaxExecutionTime:    120 * time.Second,
		MaxIterations:       30,
		CompactionThreshold: 0.85,
		CompactionKeepLast:  10,
	}
}

// RecursionState tracks accumulated context across iterations.
// Passed through context.Value so hooks and tools can read it.
type RecursionState struct {
	Iteration     int
	Depth         int
	BranchID      string
	ParentSession string
	LoopDetector  *LoopDetector
	TokensUsed    atomic.Int64
	TokenBudget   int64 // 0 = unlimited
	StartedAt     time.Time
	Aborted       atomic.Bool
}

type recursionStateKey struct{}

// WithRecursionState injects state into context.
func WithRecursionState(ctx context.Context, rs *RecursionState) context.Context {
	return context.WithValue(ctx, recursionStateKey{}, rs)
}

// StateFromContext retrieves recursion state, or nil if not in a recursion.
func StateFromContext(ctx context.Context) *RecursionState {
	rs, _ := ctx.Value(recursionStateKey{}).(*RecursionState)
	return rs
}

// RecursionResult captures the outcome of a recursion run.
type RecursionResult struct {
	Content    string
	Iterations int
	Status     RecursionStatus
	LoopInfo   *LoopResult           // Non-nil if loop was detected
	BranchResults []BranchResult     // Non-nil if MCTS was used
	Duration   time.Duration
}

type RecursionStatus string

const (
	StatusCompleted RecursionStatus = "completed"
	StatusTimeout   RecursionStatus = "timeout"
	StatusAborted   RecursionStatus = "aborted"
	StatusLoopAbort RecursionStatus = "loop_abort"
	StatusError     RecursionStatus = "error"
)

// SubTurnSpawner abstracts the ability to spawn sub-turns.
// PicoClaw's AgentLoopSpawner satisfies this via tools.SubTurnSpawner.
type SubTurnSpawner interface {
	SpawnSubTurn(ctx context.Context, cfg tools.SubTurnConfig) (*tools.ToolResult, error)
}

// LLMCaller abstracts a single LLM call for compaction.
type LLMCaller interface {
	Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition,
		model string, options map[string]any) (*providers.LLMResponse, error)
}
