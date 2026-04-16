package recursion

import (
	"context"
	"strings"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/agent"
)

type turnExecutor struct {
	cfg        RecursionConfig
	supervisor *Supervisor
}

var _ agent.TurnExecutor = (*turnExecutor)(nil)

func newTurnExecutor(cfg RecursionConfig, supervisor *Supervisor) agent.TurnExecutor {
	return &turnExecutor{cfg: cfg, supervisor: supervisor}
}

func (e *turnExecutor) PrepareTurn(info agent.TurnExecutionInfo) agent.TurnExecutionInfo {
	if info.ManagedExecution {
		if info.ExecutionReason == "" {
			info.ExecutionReason = "inherited"
		}
		return info
	}

	managed, normalized, reason := e.shouldManageTurn(info)
	if managed {
		info.ManagedExecution = true
		info.ExecutionReason = reason
		info.UserMessage = normalized
	}
	return info
}

func (e *turnExecutor) ExecuteTurn(
	ctx context.Context,
	info agent.TurnExecutionInfo,
	run func(context.Context) (string, error),
) (string, error) {
	if !info.ManagedExecution || e.supervisor == nil {
		return run(ctx)
	}

	state := &RecursionState{
		Depth:         info.Depth,
		ParentSession: info.ParentSessionKey,
		LoopDetector:  NewLoopDetector(e.cfg.LoopDetector),
		TokenBudget:   loadTokenBudget(info),
	}

	result, err := e.supervisor.Execute(ctx, info.SessionKey, state, func(execCtx context.Context, state *RecursionState) (*RecursionResult, error) {
		execCtx = agent.WithManagedTurnContext(execCtx)
		content, runErr := run(execCtx)
		if runErr != nil {
			return nil, runErr
		}

		outcome := &RecursionResult{
			Content:    content,
			Iterations: state.Iteration,
			Status:     StatusCompleted,
		}
		if state.Aborted.Load() {
			outcome.Status = StatusAborted
		}
		if state.LoopDetector != nil {
			loop := state.LoopDetector.Check()
			if loop.Detector != "" {
				outcome.LoopInfo = &loop
				if loop.Severity == SeverityCritical {
					outcome.Status = StatusLoopAbort
				}
			}
		}
		return outcome, nil
	})
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", nil
	}
	return result.Content, nil
}

func (e *turnExecutor) shouldManageTurn(info agent.TurnExecutionInfo) (bool, string, string) {
	switch e.cfg.GateMode {
	case GateModeForce:
		return true, info.UserMessage, "force"
	case GateModeManual:
		return stripManualTrigger(info.UserMessage)
	default:
		return false, info.UserMessage, ""
	}
}

func stripManualTrigger(message string) (bool, string, string) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return false, message, ""
	}

	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"/recurse", "/recursive", "/think", "recurse:", "think:"} {
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		remainder := strings.TrimSpace(trimmed[len(prefix):])
		if remainder == "" {
			remainder = trimmed
		}
		return true, remainder, "manual-prefix"
	}

	return false, message, ""
}

func loadTokenBudget(info agent.TurnExecutionInfo) int64 {
	if info.TokenBudget == nil {
		return 0
	}
	return info.TokenBudget.Load()
}