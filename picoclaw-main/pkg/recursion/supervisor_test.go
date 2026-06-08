package recursion

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSupervisor_TimeoutReportsTimeoutStatus(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxExecutionTime = 20 * time.Millisecond
	sup := NewSupervisor(cfg)

	state := &RecursionState{}
	result, err := sup.Execute(context.Background(), "sess", state,
		func(ctx context.Context, _ *RecursionState) (*RecursionResult, error) {
			<-ctx.Done() // never finishes on its own
			return nil, ctx.Err()
		})

	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if result == nil || result.Status != StatusTimeout {
		t.Fatalf("expected StatusTimeout, got %+v", result)
	}
	if !state.Aborted.Load() {
		t.Fatal("state should be marked aborted on timeout")
	}
}

func TestSupervisor_CallerCancelReportsAbortedStatus(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxExecutionTime = 5 * time.Second // long, so the deadline does not fire
	sup := NewSupervisor(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	state := &RecursionState{}
	result, err := sup.Execute(ctx, "sess", state,
		func(c context.Context, _ *RecursionState) (*RecursionResult, error) {
			<-c.Done()
			return nil, c.Err()
		})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if result == nil || result.Status != StatusAborted {
		t.Fatalf("expected StatusAborted on caller cancel, got %+v", result)
	}
}

func TestSupervisor_SuccessPropagatesResult(t *testing.T) {
	sup := NewSupervisor(DefaultConfig())
	state := &RecursionState{}
	result, err := sup.Execute(context.Background(), "sess", state,
		func(_ context.Context, _ *RecursionState) (*RecursionResult, error) {
			return &RecursionResult{Status: StatusCompleted, Content: "ok"}, nil
		})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil || result.Status != StatusCompleted || result.Content != "ok" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Duration <= 0 {
		t.Fatal("duration should be set on success")
	}
}
