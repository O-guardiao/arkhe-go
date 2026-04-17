package api

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/config"
	ppid "github.com/O-guardiao/arkhe-go/picoclaw-main/pkg/pid"
	"github.com/O-guardiao/arkhe-go/picoclaw-main/web/backend/launcherconfig"
)

func TestGatewayHostOverrideDoesNotAutoExposeGateway(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	launcherPath := launcherconfig.PathForAppConfig(configPath)
	if err := launcherconfig.Save(launcherPath, launcherconfig.Config{
		Port:   18800,
		Public: false,
	}); err != nil {
		t.Fatalf("launcherconfig.Save() error = %v", err)
	}

	h := NewHandler(configPath)
	h.SetServerOptions(18800, true, true, nil)

	if got := h.gatewayHostOverride(); got != "" {
		t.Fatalf("gatewayHostOverride() = %q, want empty", got)
	}
}

func TestBuildWsURLUsesRequestHostWhenLauncherPublicSaved(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	launcherPath := launcherconfig.PathForAppConfig(configPath)
	if err := launcherconfig.Save(launcherPath, launcherconfig.Config{
		Port:   18800,
		Public: true,
	}); err != nil {
		t.Fatalf("launcherconfig.Save() error = %v", err)
	}

	h := NewHandler(configPath)
	h.SetServerOptions(18800, false, false, nil)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 18790

	req := httptest.NewRequest("GET", "http://launcher.local/api/pico/token", nil)
	req.Host = "192.168.1.9:18800"

	if got := h.buildWsURL(req); got != "ws://192.168.1.9:18800/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "ws://192.168.1.9:18800/pico/ws")
	}

	if got := h.buildPicoEventsURL(req); got != "http://192.168.1.9:18800/pico/events" {
		t.Fatalf("buildPicoEventsURL() = %q, want %q", got, "http://192.168.1.9:18800/pico/events")
	}
	if got := h.buildPicoSendURL(req); got != "http://192.168.1.9:18800/pico/send" {
		t.Fatalf("buildPicoSendURL() = %q, want %q", got, "http://192.168.1.9:18800/pico/send")
	}
}

func TestGatewayProbeHostUsesLoopbackForWildcardBind(t *testing.T) {
	if got := gatewayProbeHost("0.0.0.0"); got != "127.0.0.1" {
		t.Fatalf("gatewayProbeHost() = %q, want %q", got, "127.0.0.1")
	}
}

func TestGatewayProxyURLUsesConfiguredHost(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "192.168.1.10"
	cfg.Gateway.Port = 18791
	if err := config.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	if got := h.gatewayProxyURL().String(); got != "http://192.168.1.10:18791" {
		t.Fatalf("gatewayProxyURL() = %q, want %q", got, "http://192.168.1.10:18791")
	}
}

func TestGetGatewayHealthUsesConfiguredHost(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "192.168.1.10"
	cfg.Gateway.Port = 18791

	originalHealthGet := gatewayHealthGet
	t.Cleanup(func() {
		gatewayHealthGet = originalHealthGet
	})

	var requestedURL string
	gatewayHealthGet = func(url string, timeout time.Duration) (*http.Response, error) {
		requestedURL = url
		return nil, errors.New("probe failed")
	}

	_, statusCode, err := h.getGatewayHealth(cfg, time.Second)
	_ = statusCode
	_ = err

	if requestedURL != "http://192.168.1.10:18791/health" {
		t.Fatalf("health url = %q, want %q", requestedURL, "http://192.168.1.10:18791/health")
	}
}

func TestGetGatewayHealthUsesProbeHostForPublicLauncher(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)
	h.SetServerOptions(18800, true, true, nil)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "127.0.0.1"
	cfg.Gateway.Port = 18791

	originalHealthGet := gatewayHealthGet
	t.Cleanup(func() {
		gatewayHealthGet = originalHealthGet
	})

	var requestedURL string
	gatewayHealthGet = func(url string, timeout time.Duration) (*http.Response, error) {
		requestedURL = url
		return nil, errors.New("probe failed")
	}

	_, statusCode, err := h.getGatewayHealth(cfg, time.Second)
	_ = statusCode
	_ = err

	if requestedURL != "http://127.0.0.1:18791/health" {
		t.Fatalf("health url = %q, want %q", requestedURL, "http://127.0.0.1:18791/health")
	}
}

func TestGetGatewayHealthUsesPidTokenForAuthorizedProbe(t *testing.T) {
	resetGatewayTestState(t)

	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	originalHealthGet := gatewayHealthGet
	originalHealthGetAuthorized := gatewayHealthGetAuthorized
	t.Cleanup(func() {
		gatewayHealthGet = originalHealthGet
		gatewayHealthGetAuthorized = originalHealthGetAuthorized
	})

	gateway.mu.Lock()
	gateway.pidData = &ppid.PidFileData{
		Host:  "127.0.0.1",
		Port:  18790,
		Token: "pid-secret",
	}
	gateway.mu.Unlock()

	var requestedURL string
	var requestedToken string
	gatewayHealthGet = func(string, time.Duration) (*http.Response, error) {
		t.Fatal("plain health probe should not be used when pid token is available")
		return nil, nil
	}
	gatewayHealthGetAuthorized = func(url, token string, timeout time.Duration) (*http.Response, error) {
		requestedURL = url
		requestedToken = token
		return nil, errors.New("probe failed")
	}

	_, _, _ = h.getGatewayHealth(config.DefaultConfig(), time.Second)

	if requestedURL != "http://127.0.0.1:18790/health" {
		t.Fatalf("health url = %q, want %q", requestedURL, "http://127.0.0.1:18790/health")
	}
	if requestedToken != "pid-secret" {
		t.Fatalf("health token = %q, want %q", requestedToken, "pid-secret")
	}
}

func TestBuildWsURLUsesWSSWhenForwardedProtoIsHTTPS(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "0.0.0.0"
	cfg.Gateway.Port = 18790

	req := httptest.NewRequest("GET", "http://launcher.local/api/pico/token", nil)
	req.Host = "chat.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")

	if got := h.buildWsURL(req); got != "wss://chat.example.com:18800/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "wss://chat.example.com:18800/pico/ws")
	}
}

func TestBuildWsURLUsesWSSWhenRequestIsTLS(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "0.0.0.0"
	cfg.Gateway.Port = 18790

	req := httptest.NewRequest("GET", "https://launcher.local/api/pico/token", nil)
	req.Host = "secure.example.com"
	req.TLS = &tls.ConnectionState{}

	if got := h.buildWsURL(req); got != "wss://secure.example.com:18800/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "wss://secure.example.com:18800/pico/ws")
	}
}

func TestBuildPicoURLsPreferXForwardedHost(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	launcherPath := launcherconfig.PathForAppConfig(configPath)
	if err := launcherconfig.Save(launcherPath, launcherconfig.Config{
		Port:   18800,
		Public: true,
	}); err != nil {
		t.Fatalf("launcherconfig.Save() error = %v", err)
	}

	h := NewHandler(configPath)
	h.SetServerOptions(18800, false, false, nil)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "0.0.0.0"
	cfg.Gateway.Port = 18790

	req := httptest.NewRequest("GET", "http://127.0.0.1:18800/api/pico/token", nil)
	req.Host = "127.0.0.1:18800"
	req.Header.Set("X-Forwarded-Host", "vscode-tunnel.example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Port", "443")

	if got := h.buildPicoEventsURL(req); got != "https://vscode-tunnel.example.com:443/pico/events" {
		t.Fatalf("buildPicoEventsURL() = %q, want %q", got, "https://vscode-tunnel.example.com:443/pico/events")
	}
	if got := h.buildPicoSendURL(req); got != "https://vscode-tunnel.example.com:443/pico/send" {
		t.Fatalf("buildPicoSendURL() = %q, want %q", got, "https://vscode-tunnel.example.com:443/pico/send")
	}
	if got := h.buildWsURL(req); got != "wss://vscode-tunnel.example.com:443/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "wss://vscode-tunnel.example.com:443/pico/ws")
	}
}

func TestBuildWsURLPrefersForwardedHTTPOverTLS(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)

	cfg := config.DefaultConfig()
	cfg.Gateway.Host = "0.0.0.0"
	cfg.Gateway.Port = 18790

	req := httptest.NewRequest("GET", "https://launcher.local/api/pico/token", nil)
	req.Host = "chat.example.com"
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("X-Forwarded-Proto", "http")

	if got := h.buildWsURL(req); got != "ws://chat.example.com:18800/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "ws://chat.example.com:18800/pico/ws")
	}
}

func TestBuildWsURLUsesRequestHostNotGatewayBindLoopback(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	h := NewHandler(configPath)
	h.SetServerOptions(18800, false, false, nil)

	req := httptest.NewRequest("GET", "http://localhost:18800/api/pico/token", nil)
	req.Host = "localhost:18800"

	if got := h.buildWsURL(req); got != "ws://localhost:18800/pico/ws" {
		t.Fatalf("buildWsURL() = %q, want %q", got, "ws://localhost:18800/pico/ws")
	}
}
