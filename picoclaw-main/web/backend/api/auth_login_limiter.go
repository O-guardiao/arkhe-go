package api

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/O-guardiao/arkhe-go/picoclaw-main/web/backend/middleware"
)

const (
	loginAttemptsPerIP = 10
	loginAttemptWindow = time.Minute
	logoutBodyMaxBytes = 4096
	rateLimitStateFile = "rate_limit_state.json"
)

// loginRateLimiter limits POST /api/auth/login attempts per IP per minute.
// State is persisted to disk so that it survives process restarts (VULN-005).
type loginRateLimiter struct {
	mu                sync.Mutex
	now               func() time.Time
	byIP              map[string][]time.Time
	trustedProxyCIDRs []*net.IPNet
	persistPath       string // empty = no persistence (tests)
}

// rateLimitSnapshot is the JSON-serializable representation of rate limiter state.
type rateLimitSnapshot struct {
	ByIP map[string][]time.Time `json:"by_ip"`
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{
		now:  time.Now,
		byIP: make(map[string][]time.Time),
	}
}

// newPersistentLoginRateLimiter creates a rate limiter that persists state to dataDir.
func newPersistentLoginRateLimiter(dataDir string) *loginRateLimiter {
	l := &loginRateLimiter{
		now:         time.Now,
		byIP:        make(map[string][]time.Time),
		persistPath: filepath.Join(dataDir, rateLimitStateFile),
	}
	l.loadState()
	return l
}

// loadState restores rate limiter state from disk, pruning expired entries.
func (l *loginRateLimiter) loadState() {
	if l.persistPath == "" {
		return
	}
	data, err := os.ReadFile(l.persistPath)
	if err != nil {
		return // file doesn't exist yet — normal on first run
	}
	var snap rateLimitSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return // corrupt file — start fresh
	}
	cutoff := l.now().Add(-loginAttemptWindow)
	for ip, times := range snap.ByIP {
		var kept []time.Time
		for _, ts := range times {
			if ts.After(cutoff) {
				kept = append(kept, ts)
			}
		}
		if len(kept) > 0 {
			l.byIP[ip] = kept
		}
	}
}

// SaveState persists the current rate limiter state to disk.
// Called on graceful shutdown and periodically.
func (l *loginRateLimiter) SaveState() {
	if l.persistPath == "" {
		return
	}
	l.mu.Lock()
	snap := rateLimitSnapshot{ByIP: make(map[string][]time.Time, len(l.byIP))}
	for ip, times := range l.byIP {
		snap.ByIP[ip] = times
	}
	l.mu.Unlock()

	data, err := json.Marshal(&snap)
	if err != nil {
		return
	}
	// Write atomically: write to temp file, then rename.
	tmp := l.persistPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return
	}
	_ = os.Rename(tmp, l.persistPath)
}

// allow reserves a slot for this request; false means rate limit exceeded.
func (l *loginRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	cutoff := now.Add(-loginAttemptWindow)
	times := l.byIP[ip]
	var kept []time.Time
	for _, ts := range times {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= loginAttemptsPerIP {
		l.byIP[ip] = kept
		return false
	}
	kept = append(kept, now)
	l.byIP[ip] = kept
	return true
}

func clientIPForLimiter(r *http.Request) string {
	ip := middleware.RealIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"), nil)
	if ip == nil {
		return r.RemoteAddr
	}
	return ip.String()
}
