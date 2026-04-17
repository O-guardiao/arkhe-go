package recursion

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/tools"
)

type benchmarkSpawner struct{}

func (benchmarkSpawner) SpawnSubTurn(ctx context.Context, cfg tools.SubTurnConfig) (*tools.ToolResult, error) {
	if strings.Contains(cfg.SystemPrompt, "Previous candidate") {
		return tools.NewToolResult("refined candidate with concrete validation 123 456 and implementation detail"), nil
	}

	switch {
	case strings.Contains(cfg.ActualSystemPrompt, "Strategy: planner"):
		return tools.NewToolResult("planner candidate 123 with explicit decomposition"), nil
	case strings.Contains(cfg.ActualSystemPrompt, "Strategy: critic"):
		return tools.NewToolResult("critic candidate 456 with risk checks"), nil
	case strings.Contains(cfg.ActualSystemPrompt, "Strategy: verify"):
		return tools.NewToolResult("verification candidate 789 with acceptance checks"), nil
	default:
		return tools.NewToolResult("direct candidate with concrete detail and safe completion"), nil
	}
}

func BenchmarkRunMCTS(b *testing.B) {
	task := "Solve the task completely with a concrete, validated answer."
	spawner := benchmarkSpawner{}

	benchmarks := []struct {
		name string
		cfg  MCTSConfig
	}{
		{
			name: "branches_3_depth_1",
			cfg: MCTSConfig{
				Branches: 3,
				Depth:    1,
				Timeout:  time.Second,
				Model:    "bench-model",
			},
		},
		{
			name: "branches_3_depth_2",
			cfg: MCTSConfig{
				Branches: 3,
				Depth:    2,
				Timeout:  time.Second,
				Model:    "bench-model",
			},
		},
		{
			name: "branches_6_depth_3",
			cfg: MCTSConfig{
				Branches: 6,
				Depth:    3,
				Timeout:  time.Second,
				Model:    "bench-model",
			},
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			ctx := context.Background()

			for i := 0; i < b.N; i++ {
				winner, all, err := RunMCTS(ctx, spawner, task, bm.cfg)
				if err != nil {
					b.Fatalf("RunMCTS returned error: %v", err)
				}
				if winner == nil || len(all) == 0 {
					b.Fatal("RunMCTS returned empty result")
				}
			}
		})
	}
}

