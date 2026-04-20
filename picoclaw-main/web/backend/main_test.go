package main

import (
	"path/filepath"
	"testing"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/web/backend/launcherconfig"
)

func TestShouldEnableLauncherFileLogging(t *testing.T) {
	tests := []struct {
		name          string
		enableConsole bool
		debug         bool
		want          bool
	}{
		{name: "gui mode", enableConsole: false, debug: false, want: true},
		{name: "console mode", enableConsole: true, debug: false, want: false},
		{name: "debug gui mode", enableConsole: false, debug: true, want: true},
		{name: "debug console mode", enableConsole: true, debug: true, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldEnableLauncherFileLogging(tt.enableConsole, tt.debug); got != tt.want {
				t.Fatalf(
					"shouldEnableLauncherFileLogging(%t, %t) = %t, want %t",
					tt.enableConsole,
					tt.debug,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestDashboardTokenConfigHelpPath(t *testing.T) {
	const launcherPath = "/tmp/launcher-config.json"

	tests := []struct {
		name   string
		source launcherconfig.DashboardTokenSource
		want   string
	}{
		{
			name:   "env token does not expose config path",
			source: launcherconfig.DashboardTokenSourceEnv,
			want:   "",
		},
		{
			name:   "config token exposes config path",
			source: launcherconfig.DashboardTokenSourceConfig,
			want:   launcherPath,
		},
		{
			name:   "random token does not expose config path",
			source: launcherconfig.DashboardTokenSourceRandom,
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dashboardTokenConfigHelpPath(tt.source, launcherPath); got != tt.want {
				t.Fatalf("dashboardTokenConfigHelpPath(%q, %q) = %q, want %q", tt.source, launcherPath, got, tt.want)
			}
		})
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Long token (>=12 chars): first 3 + 10 stars + last 4
		{"sdhjflsjdflksdf", "sdh**********ksdf"},
		{"abcdefghijklmnopqrstuvwxyz", "abc**********wxyz"},
		// Exactly 12 chars (3+4+5 hidden): suffix shown
		{"abcdefghijkl", "abc**********ijkl"},
		// 8 chars (minimum password length): suffix NOT shown — only prefix+stars
		{"abcdefgh", "abc**********"},
		// 11 chars (one below threshold): suffix NOT shown
		{"abcdefghijk", "abc**********"},
		// 4..3 chars: prefix shown, no suffix
		{"abcdefg", "abc**********"},
		{"abcd", "abc**********"},
		// <=3 chars: fully masked
		{"abc", "**********"},
		{"", "**********"},
	}
	for _, tt := range tests {
		if got := maskSecret(tt.input); got != tt.want {
			t.Errorf("maskSecret(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeLauncherRuntimeConfigMigratesConflictingStoredPort(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	launcherPath := filepath.Join(dir, launcherconfig.FileName)

	if err := config.SaveConfig(configPath, config.DefaultConfig()); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	if err := launcherconfig.Save(launcherPath, launcherconfig.Config{
		Port:   18790,
		Public: false,
	}); err != nil {
		t.Fatalf("launcherconfig.Save() error = %v", err)
	}

	got, warning, err := normalizeLauncherRuntimeConfig(
		configPath,
		launcherPath,
		launcherconfig.Config{Port: 18790, Public: false},
		false,
	)
	if err != nil {
		t.Fatalf("normalizeLauncherRuntimeConfig() error = %v", err)
	}
	if got.Port != launcherconfig.DefaultPort {
		t.Fatalf("normalized port = %d, want %d", got.Port, launcherconfig.DefaultPort)
	}
	if warning == "" {
		t.Fatal("expected migration warning for conflicting stored port")
	}

	saved, err := launcherconfig.Load(launcherPath, launcherconfig.Default())
	if err != nil {
		t.Fatalf("launcherconfig.Load() error = %v", err)
	}
	if saved.Port != launcherconfig.DefaultPort {
		t.Fatalf("saved launcher port = %d, want %d", saved.Port, launcherconfig.DefaultPort)
	}
}

func TestNormalizeLauncherRuntimeConfigKeepsNonConflictingPort(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	launcherPath := filepath.Join(dir, launcherconfig.FileName)

	cfg := config.DefaultConfig()
	cfg.Gateway.Port = 18801
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	got, warning, err := normalizeLauncherRuntimeConfig(
		configPath,
		launcherPath,
		launcherconfig.Config{Port: 18800, Public: false},
		false,
	)
	if err != nil {
		t.Fatalf("normalizeLauncherRuntimeConfig() error = %v", err)
	}
	if got.Port != 18800 {
		t.Fatalf("normalized port = %d, want %d", got.Port, 18800)
	}
	if warning != "" {
		t.Fatalf("warning = %q, want empty", warning)
	}
}
