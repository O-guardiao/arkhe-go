package agent

import (
	"context"
	"sync/atomic"
)

// TurnExecutionInfo is the normalized contract exposed to turn executors that
// want to supervise or decorate a full AgentLoop turn without owning the
// operational shell around it.
type TurnExecutionInfo struct {
	AgentID          string
	SessionKey       string
	ParentSessionKey string
	Channel          string
	ChatID           string
	UserMessage      string
	Workspace        string
	Model            string
	Depth            int
	TokenBudget      *atomic.Int64

	ManagedExecution bool
	ExecutionReason  string
}

// TurnExecutor can normalize an incoming turn request and optionally wrap its
// execution with additional context, supervision, budgeting or lifecycle rules.
type TurnExecutor interface {
	PrepareTurn(info TurnExecutionInfo) TurnExecutionInfo
	ExecuteTurn(ctx context.Context, info TurnExecutionInfo, run func(context.Context) (string, error)) (string, error)
}

type managedTurnKey struct{}

// WithManagedTurnContext marks a context as already executing under a managed
// turn runtime so child turns can inherit the same execution mode safely.
func WithManagedTurnContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, managedTurnKey{}, true)
}

// ManagedTurnContextActive reports whether the current context is already
// running under a managed turn executor.
func ManagedTurnContextActive(ctx context.Context) bool {
	active, _ := ctx.Value(managedTurnKey{}).(bool)
	return active
}

// SetTurnExecutor installs the top-level turn executor used by runAgentLoop.
func (al *AgentLoop) SetTurnExecutor(executor TurnExecutor) {
	if al == nil {
		return
	}
	al.mu.Lock()
	defer al.mu.Unlock()
	al.turnExecutor = executor
}

func (al *AgentLoop) getTurnExecutor() TurnExecutor {
	if al == nil {
		return nil
	}
	al.mu.RLock()
	defer al.mu.RUnlock()
	return al.turnExecutor
}

func (al *AgentLoop) buildTurnExecutionInfo(
	ctx context.Context,
	agent *AgentInstance,
	opts processOptions,
) TurnExecutionInfo {
	info := TurnExecutionInfo{
		SessionKey:  opts.SessionKey,
		Channel:     opts.Channel,
		ChatID:      opts.ChatID,
		UserMessage: opts.UserMessage,
	}
	if agent != nil {
		info.AgentID = agent.ID
		info.Workspace = agent.Workspace
		info.Model = agent.Model
	}
	if parentTS := turnStateFromContext(ctx); parentTS != nil {
		info.Depth = parentTS.depth + 1
		info.ParentSessionKey = parentTS.sessionKey
		info.TokenBudget = parentTS.tokenBudget
	}
	if ManagedTurnContextActive(ctx) {
		info.ManagedExecution = true
		info.ExecutionReason = "inherited"
	}
	return info
}