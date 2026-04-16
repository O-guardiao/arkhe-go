package recursion

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/agent"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/bus"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/providers"
)

type recursionTestProvider struct {
	response string
	delay    time.Duration
	lastUser string
	calls    atomic.Int32
}

func (p *recursionTestProvider) Chat(
	ctx context.Context,
	messages []providers.Message,
	_ []providers.ToolDefinition,
	_ string,
	_ map[string]any,
) (*providers.LLMResponse, error) {
	p.calls.Add(1)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			p.lastUser = messages[i].Content
			break
		}
	}
	if p.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(p.delay):
		}
	}
	return &providers.LLMResponse{Content: p.response}, nil
}

func (p *recursionTestProvider) GetDefaultModel() string {
	return "mock-model"
}

type recursionStateHook struct {
	sawState   bool
	sawManaged bool
	depth      int
	tokens     int64
	callCount  int
}

var _ agent.LLMInterceptor = (*recursionStateHook)(nil)

func (h *recursionStateHook) BeforeLLM(
	ctx context.Context,
	req *agent.LLMHookRequest,
) (*agent.LLMHookRequest, agent.HookDecision, error) {
	h.callCount++
	h.sawManaged = agent.ManagedTurnContextActive(ctx)
	if rs := StateFromContext(ctx); rs != nil {
		h.sawState = true
		h.depth = rs.Depth
		h.tokens = rs.TokenBudget
	}
	return req, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

func (h *recursionStateHook) AfterLLM(
	ctx context.Context,
	resp *agent.LLMHookResponse,
) (*agent.LLMHookResponse, agent.HookDecision, error) {
	return resp, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

type noopToolInterceptor struct{}

var _ agent.ToolInterceptor = (*noopToolInterceptor)(nil)

func (noopToolInterceptor) BeforeTool(
	ctx context.Context,
	call *agent.ToolCallHookRequest,
) (*agent.ToolCallHookRequest, agent.HookDecision, error) {
	return call, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

func (noopToolInterceptor) AfterTool(
	ctx context.Context,
	resp *agent.ToolResultHookResponse,
) (*agent.ToolResultHookResponse, agent.HookDecision, error) {
	return resp, agent.HookDecision{Action: agent.HookActionContinue}, nil
}

func newRecursionTestLoop(
	t *testing.T,
	provider providers.LLMProvider,
	recCfg config.RecursionConfig,
) *agent.AgentLoop {
	t.Helper()
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         t.TempDir(),
				ModelName:         "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		Recursion: recCfg,
	}
	msgBus := bus.NewMessageBus()
	loop := agent.NewAgentLoop(cfg, msgBus, provider)
	Setup(loop)
	t.Cleanup(func() {
		loop.Close()
		msgBus.Close()
	})
	return loop
}

func TestRecursionSetup_ForceModeInjectsManagedState(t *testing.T) {
	provider := &recursionTestProvider{response: "ok"}
	loop := newRecursionTestLoop(t, provider, config.RecursionConfig{
		Enabled:  true,
		GateMode: "force",
	})

	hook := &recursionStateHook{}
	if err := loop.MountHook(agent.HookRegistration{
		Name:     "recursion-state-test",
		Priority: 50,
		Source:   agent.HookSourceInProcess,
		Hook:     hook,
	}); err != nil {
		t.Fatalf("MountHook(): %v", err)
	}

	response, err := loop.ProcessDirect(context.Background(), "hello world", "cli:force")
	if err != nil {
		t.Fatalf("ProcessDirect(): %v", err)
	}
	if response != "ok" {
		t.Fatalf("response = %q, want ok", response)
	}
	if !hook.sawManaged {
		t.Fatal("expected managed turn context to be active")
	}
	if !hook.sawState {
		t.Fatal("expected recursion state to be injected into LLM hook context")
	}
	if hook.depth != 0 {
		t.Fatalf("hook.depth = %d, want 0", hook.depth)
	}
}

func TestRecursionSetup_ManualModeStripsTriggerPrefix(t *testing.T) {
	provider := &recursionTestProvider{response: "ok"}
	loop := newRecursionTestLoop(t, provider, config.RecursionConfig{
		Enabled:  true,
		GateMode: "manual",
	})

	hook := &recursionStateHook{}
	if err := loop.MountHook(agent.HookRegistration{
		Name:     "recursion-manual-state-test",
		Priority: 50,
		Source:   agent.HookSourceInProcess,
		Hook:     hook,
	}); err != nil {
		t.Fatalf("MountHook(): %v", err)
	}

	response, err := loop.ProcessDirect(context.Background(), "/recurse inspect the workspace", "cli:manual")
	if err != nil {
		t.Fatalf("ProcessDirect(): %v", err)
	}
	if response != "ok" {
		t.Fatalf("response = %q, want ok", response)
	}
	if provider.lastUser != "inspect the workspace" {
		t.Fatalf("provider.lastUser = %q, want stripped recursion payload", provider.lastUser)
	}
	if !hook.sawManaged || !hook.sawState {
		t.Fatalf("expected manual trigger to enable managed recursion, sawManaged=%v sawState=%v", hook.sawManaged, hook.sawState)
	}
}

func TestRecursionSetup_ManualModeLeavesNormalTurnsUntouched(t *testing.T) {
	provider := &recursionTestProvider{response: "ok"}
	loop := newRecursionTestLoop(t, provider, config.RecursionConfig{
		Enabled:  true,
		GateMode: "manual",
	})

	hook := &recursionStateHook{}
	if err := loop.MountHook(agent.HookRegistration{
		Name:     "recursion-manual-off-test",
		Priority: 50,
		Source:   agent.HookSourceInProcess,
		Hook:     hook,
	}); err != nil {
		t.Fatalf("MountHook(): %v", err)
	}

	response, err := loop.ProcessDirect(context.Background(), "plain request", "cli:plain")
	if err != nil {
		t.Fatalf("ProcessDirect(): %v", err)
	}
	if response != "ok" {
		t.Fatalf("response = %q, want ok", response)
	}
	if provider.lastUser != "plain request" {
		t.Fatalf("provider.lastUser = %q, want plain request", provider.lastUser)
	}
	if hook.sawManaged || hook.sawState {
		t.Fatalf("expected normal manual-mode turn to stay on normal path, sawManaged=%v sawState=%v", hook.sawManaged, hook.sawState)
	}
}

func TestRecursionSetup_ForceModeSupervisesTimeout(t *testing.T) {
	provider := &recursionTestProvider{
		response: "late",
		delay:    1500 * time.Millisecond,
	}
	loop := newRecursionTestLoop(t, provider, config.RecursionConfig{
		Enabled:             true,
		GateMode:            "force",
		MaxExecutionTimeSec: 1,
	})

	_, err := loop.ProcessDirect(context.Background(), "slow request", "cli:timeout")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "recursion timeout after") {
		t.Fatalf("error = %q, want recursion timeout", err.Error())
	}
	if provider.calls.Load() != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls.Load())
	}
}
