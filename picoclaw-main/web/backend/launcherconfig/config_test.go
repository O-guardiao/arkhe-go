package launcherconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/web/backend/middleware"
)

func TestLoadReturnsFallbackWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "launcher-config.json")
	fallback := Config{Port: 19999, Public: true}

	got, err := Load(path, fallback)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Port != fallback.Port || got.Public != fallback.Public {
		t.Fatalf("Load() = %+v, want %+v", got, fallback)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "launcher-config.json")
	want := Config{
		Port:          18080,
		Public:        true,
		AllowedCIDRs:  []string{"192.168.1.0/24", "10.0.0.0/8"},
		LauncherToken: "saved-launcher-token",
	}

	if err := Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := Load(path, Default())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Port != want.Port || got.Public != want.Public {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if got.LauncherToken != want.LauncherToken {
		t.Fatalf("launcher_token = %q, want %q", got.LauncherToken, want.LauncherToken)
	}
	if len(got.AllowedCIDRs) != len(want.AllowedCIDRs) {
		t.Fatalf("allowed_cidrs len = %d, want %d", len(got.AllowedCIDRs), len(want.AllowedCIDRs))
	}
	for i := range want.AllowedCIDRs {
		if got.AllowedCIDRs[i] != want.AllowedCIDRs[i] {
			t.Fatalf("allowed_cidrs[%d] = %q, want %q", i, got.AllowedCIDRs[i], want.AllowedCIDRs[i])
		}
	}

	stat, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := stat.Mode().Perm(); perm != 0o600 {
			t.Fatalf("file perm = %o, want 600", perm)
		}
	}
}

func TestValidateRejectsInvalidPort(t *testing.T) {
	if err := Validate(Config{Port: 0, Public: false}); err == nil {
		t.Fatal("Validate() expected error for port 0")
	}
	if err := Validate(Config{Port: 65536, Public: false}); err == nil {
		t.Fatal("Validate() expected error for port 65536")
	}
}

func TestValidateRejectsInvalidCIDR(t *testing.T) {
	err := Validate(Config{
		Port:         18800,
		AllowedCIDRs: []string{"192.168.1.0/24", "not-a-cidr"},
	})
	if err == nil {
		t.Fatal("Validate() expected error for invalid CIDR")
	}
}

func TestPortConflictsWithGateway(t *testing.T) {
	tests := []struct {
		name           string
		launcherPort   int
		launcherPublic bool
		gatewayHost    string
		gatewayPort    int
		want           bool
	}{
		{
			name:           "different ports do not conflict",
			launcherPort:   18800,
			launcherPublic: false,
			gatewayHost:    "127.0.0.1",
			gatewayPort:    18790,
			want:           false,
		},
		{
			name:           "public launcher conflicts on same port",
			launcherPort:   18790,
			launcherPublic: true,
			gatewayHost:    "127.0.0.1",
			gatewayPort:    18790,
			want:           true,
		},
		{
			name:           "loopback launcher conflicts with default gateway host",
			launcherPort:   18790,
			launcherPublic: false,
			gatewayHost:    "",
			gatewayPort:    18790,
			want:           true,
		},
		{
			name:           "custom gateway host avoids loopback conflict",
			launcherPort:   18790,
			launcherPublic: false,
			gatewayHost:    "10.10.10.10",
			gatewayPort:    18790,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PortConflictsWithGateway(tt.launcherPort, tt.launcherPublic, tt.gatewayHost, tt.gatewayPort); got != tt.want {
				t.Fatalf(
					"PortConflictsWithGateway(%d, %t, %q, %d) = %v, want %v",
					tt.launcherPort,
					tt.launcherPublic,
					tt.gatewayHost,
					tt.gatewayPort,
					got,
					tt.want,
				)
			}
		})
	}
}

func TestGatewayHostLabel(t *testing.T) {
	if got := GatewayHostLabel(""); got != "127.0.0.1" {
		t.Fatalf("GatewayHostLabel(\"\") = %q, want %q", got, "127.0.0.1")
	}
	if got := GatewayHostLabel(" 0.0.0.0 "); got != "0.0.0.0" {
		t.Fatalf("GatewayHostLabel(\" 0.0.0.0 \") = %q, want %q", got, "0.0.0.0")
	}
}

func TestNextSafePort(t *testing.T) {
	if got := NextSafePort(18790); got != 18800 {
		t.Fatalf("NextSafePort(18790) = %d, want %d", got, 18800)
	}
	if got := NextSafePort(18800); got != 18801 {
		t.Fatalf("NextSafePort(18800) = %d, want %d", got, 18801)
	}
}

func TestEnsureDashboardSecrets_GeneratesEphemeral(t *testing.T) {
	t.Setenv("PICOCLAW_LAUNCHER_TOKEN", "")

	tok, key, source, err := EnsureDashboardSecrets(Default())
	if err != nil {
		t.Fatalf("EnsureDashboardSecrets() error = %v", err)
	}
	if source != DashboardTokenSourceRandom || tok == "" || len(key) != dashboardSigningKeyBytes {
		t.Fatalf("unexpected first call: source=%q tok=%q keyLen=%d", source, tok, len(key))
	}
	mac := middleware.SessionCookieValue(key, tok)
	if mac == "" {
		t.Fatal("empty session mac")
	}

	tok2, key2, source2, err := EnsureDashboardSecrets(Default())
	if err != nil {
		t.Fatalf("EnsureDashboardSecrets() second error = %v", err)
	}
	if source2 != DashboardTokenSourceRandom {
		t.Fatalf("second call source = %q, want %q", source2, DashboardTokenSourceRandom)
	}
	if tok2 == tok {
		t.Fatal("expected a new random dashboard token")
	}
	if string(key2) == string(key) {
		t.Fatal("expected a new signing key")
	}
}

func TestEnsureDashboardSecrets_EnvOverridesGenerated(t *testing.T) {
	t.Setenv("PICOCLAW_LAUNCHER_TOKEN", "env-only-token-override")

	tok, _, source, err := EnsureDashboardSecrets(Config{LauncherToken: "config-token"})
	if err != nil {
		t.Fatalf("EnsureDashboardSecrets() error = %v", err)
	}
	if tok != "env-only-token-override" {
		t.Fatalf("token = %q, want env value", tok)
	}
	if source != DashboardTokenSourceEnv {
		t.Fatalf("source = %q, want %q", source, DashboardTokenSourceEnv)
	}
}

func TestEnsureDashboardSecrets_ConfigOverridesGenerated(t *testing.T) {
	t.Setenv("PICOCLAW_LAUNCHER_TOKEN", "")

	tok, _, source, err := EnsureDashboardSecrets(Config{LauncherToken: "config-token"})
	if err != nil {
		t.Fatalf("EnsureDashboardSecrets() error = %v", err)
	}
	if tok != "config-token" {
		t.Fatalf("token = %q, want config value", tok)
	}
	if source != DashboardTokenSourceConfig {
		t.Fatalf("source = %q, want %q", source, DashboardTokenSourceConfig)
	}
}

func TestNormalizeCIDRs(t *testing.T) {
	got := NormalizeCIDRs([]string{" 192.168.1.0/24 ", "", "10.0.0.0/8", "192.168.1.0/24"})
	want := []string{"192.168.1.0/24", "10.0.0.0/8"}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
