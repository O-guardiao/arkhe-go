package recursion

import (
	"time"

	"github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// Setup integrates the recursion engine with an existing AgentLoop.
// It reads Recursion config, mounts the RecursionHook as a ToolInterceptor
// + LLMInterceptor, registers the MCTSTool on all agents, and creates a
// Supervisor. Call this after NewAgentLoop().
//
// If recursion is not enabled in config, this is a no-op.
func Setup(al *agent.AgentLoop) *Supervisor {
	cfg := al.GetConfig()
	if cfg == nil || !cfg.Recursion.Enabled {
		return nil
	}

	rc := configFromSettings(cfg.Recursion)

	// 1. Mount the RecursionHook as in-process hook.
	hook := NewRecursionHook(rc)
	if err := al.MountHook(agent.HookRegistration{
		Name:     "recursion",
		Priority: 10,
		Source:   agent.HookSourceInProcess,
		Hook:     hook,
	}); err != nil {
		logger.ErrorCF("recursion", "Failed to mount recursion hook", map[string]any{"error": err.Error()})
		return nil
	}

	// 2. Register MCTSTool on all agents.
	registry := al.GetRegistry()
	spawner := agent.NewSubTurnSpawner(al)
	mctsCfg := MCTSConfig{
		Branches: rc.MCTSBranches,
		Depth:    rc.MCTSDepth,
		Timeout:  rc.MCTSTimeout,
	}
	for _, agentID := range registry.ListAgentIDs() {
		agentInst, ok := registry.GetAgent(agentID)
		if !ok {
			continue
		}
		tool := NewMCTSTool(spawner, agentInst.Model, mctsCfg)
		agentInst.Tools.Register(tool)
	}

	// 3. Create the Supervisor.
	supervisor := NewSupervisor(rc)

	logger.InfoCF("recursion", "Recursion engine initialized", map[string]any{
		"mcts_branches":    rc.MCTSBranches,
		"max_iterations":   rc.MaxIterations,
		"max_exec_time_s":  int(rc.MaxExecutionTime.Seconds()),
	})

	return supervisor
}

// configFromSettings converts config.RecursionConfig (plain types) to the
// internal RecursionConfig with proper durations and defaults.
func configFromSettings(s config.RecursionConfig) RecursionConfig {
	rc := DefaultConfig()

	if s.MCTSBranches > 0 {
		rc.MCTSBranches = s.MCTSBranches
	}
	if s.MCTSDepthPerBranch > 0 {
		rc.MCTSDepth = s.MCTSDepthPerBranch
	}
	if s.MCTSTimeoutSec > 0 {
		rc.MCTSTimeout = time.Duration(s.MCTSTimeoutSec) * time.Second
	}
	if s.MaxExecutionTimeSec > 0 {
		rc.MaxExecutionTime = time.Duration(s.MaxExecutionTimeSec) * time.Second
	}
	if s.MaxIterations > 0 {
		rc.MaxIterations = s.MaxIterations
	}
	if s.CompactionThreshold > 0 && s.CompactionThreshold <= 100 {
		rc.CompactionThreshold = float64(s.CompactionThreshold) / 100.0
	}
	if s.CompactionKeepLast > 0 {
		rc.CompactionKeepLast = s.CompactionKeepLast
	}

	return rc
}
