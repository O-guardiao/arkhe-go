package recursion

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Supervisor guards a recursion execution with per-session locking,
// timeout enforcement, and loop detection integration.
// One Supervisor per AgentLoop; multiple sessions managed concurrently.
type Supervisor struct {
	mu       sync.Mutex
	sessions map[string]*sessionLock
	cfg      RecursionConfig
}

type sessionLock struct {
	mu       sync.Mutex
	running  bool
	cancelFn context.CancelFunc
}

func NewSupervisor(cfg RecursionConfig) *Supervisor {
	return &Supervisor{
		sessions: make(map[string]*sessionLock),
		cfg:      cfg,
	}
}

// Execute runs fn under supervision for the given sessionKey.
// Only one execution per session at a time (serialized, not dropped).
// Returns the recursion result or an error with status context.
func (s *Supervisor) Execute(
	ctx context.Context,
	sessionKey string,
	state *RecursionState,
	fn func(ctx context.Context, state *RecursionState) (*RecursionResult, error),
) (*RecursionResult, error) {
	sl := s.getOrCreateLock(sessionKey)

	// Per-session serialization
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if state.Aborted.Load() {
		return &RecursionResult{Status: StatusAborted}, nil
	}

	// Apply timeout
	timeout := s.cfg.MaxExecutionTime
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sl.running = true
	sl.cancelFn = cancel
	defer func() {
		sl.running = false
		sl.cancelFn = nil
	}()

	// Inject state into context
	execCtx = WithRecursionState(execCtx, state)
	state.StartedAt = time.Now()

	// Run the actual work
	resultCh := make(chan execOutcome, 1)
	go func() {
		r, err := fn(execCtx, state)
		resultCh <- execOutcome{result: r, err: err}
	}()

	select {
	case outcome := <-resultCh:
		if outcome.err != nil {
			return &RecursionResult{
				Status:   StatusError,
				Duration: time.Since(state.StartedAt),
			}, outcome.err
		}
		// Check if loop detection triggered abort
		if outcome.result != nil && outcome.result.LoopInfo != nil &&
			outcome.result.LoopInfo.Severity == SeverityCritical {
			outcome.result.Status = StatusLoopAbort
		}
		if outcome.result != nil {
			outcome.result.Duration = time.Since(state.StartedAt)
		}
		return outcome.result, nil

	case <-execCtx.Done():
		state.Aborted.Store(true)
		return &RecursionResult{
			Status:   StatusTimeout,
			Duration: time.Since(state.StartedAt),
		}, fmt.Errorf("recursion timeout after %v", timeout)
	}
}

// Abort cancels running execution for a session.
func (s *Supervisor) Abort(sessionKey string) bool {
	s.mu.Lock()
	sl, ok := s.sessions[sessionKey]
	s.mu.Unlock()
	if !ok {
		return false
	}

	sl.mu.Lock()
	defer sl.mu.Unlock()
	if sl.cancelFn != nil {
		sl.cancelFn()
		return true
	}
	return false
}

// IsRunning checks if a session has an active recursion.
func (s *Supervisor) IsRunning(sessionKey string) bool {
	s.mu.Lock()
	sl, ok := s.sessions[sessionKey]
	s.mu.Unlock()
	if !ok {
		return false
	}
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.running
}

func (s *Supervisor) getOrCreateLock(key string) *sessionLock {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sl, ok := s.sessions[key]; ok {
		return sl
	}
	sl := &sessionLock{}
	s.sessions[key] = sl
	return sl
}

type execOutcome struct {
	result *RecursionResult
	err    error
}
