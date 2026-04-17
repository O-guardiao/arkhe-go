package agent

import (
	"context"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
)

func init() {
	// Register the memory_extractor builtin hook.
	// Activated via config:
	//   "hooks": { "enabled": true, "builtins": { "memory_extractor": { "enabled": true } } }
	_ = RegisterWorkspaceHook("memory_extractor", func(
		ctx context.Context,
		spec config.BuiltinHookConfig,
		workspace string,
	) (any, error) {
		return newMemoryExtractorHook(ctx, workspace)
	})
}
