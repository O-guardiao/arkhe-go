package internal

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
)

func TestGetConfigPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv(config.EnvHome, filepath.Join(home, ".picoclaw"))

	got := GetConfigPath()
	want := filepath.Join(home, ".picoclaw", "config.json")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithPICOCLAW_HOME(t *testing.T) {
	home := filepath.Join(t.TempDir(), "picoclaw")
	t.Setenv(config.EnvHome, home)

	got := GetConfigPath()
	want := filepath.Join(home, "config.json")

	assert.Equal(t, want, got)
}

func TestGetConfigPath_WithPICOCLAW_CONFIG(t *testing.T) {
	customConfig := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PICOCLAW_CONFIG", customConfig)
	t.Setenv(config.EnvHome, filepath.Join(t.TempDir(), "picoclaw"))

	got := GetConfigPath()

	assert.Equal(t, customConfig, got)
}

func TestGetConfigPath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-specific HOME behavior varies; run on windows")
	}

	testUserProfilePath := `C:\Users\Test`
	t.Setenv("USERPROFILE", testUserProfilePath)

	got := GetConfigPath()
	want := filepath.Join(testUserProfilePath, ".picoclaw", "config.json")

	require.True(t, strings.EqualFold(got, want), "GetConfigPath() = %q, want %q", got, want)
}
