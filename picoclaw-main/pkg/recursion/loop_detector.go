package recursion

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

// Severity classifies the urgency of a loop detection.
type Severity uint8

const (
	SeverityOK       Severity = iota // No loop detected
	SeverityWarning                  // Pattern emerging, continue with caution
	SeverityCritical                 // Hard loop, should abort
)

// DetectorKind identifies which detector triggered.
type DetectorKind string

const (
	DetectorRepeat   DetectorKind = "generic_repeat"
	DetectorPingPong DetectorKind = "ping_pong"
	DetectorStall    DetectorKind = "no_progress"
)

// LoopResult carries the outcome of a loop check.
type LoopResult struct {
	Stuck    bool
	Severity Severity
	Detector DetectorKind
	Count    int
	Message  string
}

// OK returns true when no loop is detected.
func (r LoopResult) OK() bool { return !r.Stuck }

// LoopDetectorConfig tunes thresholds for the three detectors.
type LoopDetectorConfig struct {
	WindowSize          int // Sliding window entries (default 30)
	RepeatWarning       int // Identical code streak for warning (default 10)
	RepeatCritical      int // Identical code streak for abort (default 20)
	PingPongWarning     int // Alternating pairs for warning (default 10)
	PingPongCritical    int // Alternating pairs for abort (default 20)
	StallWarning        int // Identical output streak for warning (default 5)
	StallCritical       int // Identical output streak for abort (default 10)
}

func DefaultLoopDetectorConfig() LoopDetectorConfig {
	return LoopDetectorConfig{
		WindowSize:       30,
		RepeatWarning:    10,
		RepeatCritical:   20,
		PingPongWarning:  10,
		PingPongCritical: 20,
		StallWarning:     5,
		StallCritical:    10,
	}
}

type entry struct {
	codeHash   string
	outputHash string
}

// LoopDetector tracks tool-call/response patterns to detect stuck loops.
// Thread-safe for concurrent hook invocations.
type LoopDetector struct {
	mu      sync.Mutex
	cfg     LoopDetectorConfig
	entries []entry
}

func NewLoopDetector(cfg LoopDetectorConfig) *LoopDetector {
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 30
	}
	return &LoopDetector{cfg: cfg, entries: make([]entry, 0, cfg.WindowSize)}
}

// Record adds an observation. code is the tool call or LLM response;
// output is the tool result or execution output.
func (ld *LoopDetector) Record(code, output string) {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	e := entry{codeHash: quickHash(code), outputHash: quickHash(output)}
	ld.entries = append(ld.entries, e)
	if len(ld.entries) > ld.cfg.WindowSize {
		ld.entries = ld.entries[len(ld.entries)-ld.cfg.WindowSize:]
	}
}

// Check runs all three detectors and returns the worst result.
func (ld *LoopDetector) Check() LoopResult {
	ld.mu.Lock()
	defer ld.mu.Unlock()

	if len(ld.entries) < 3 {
		return LoopResult{}
	}

	worst := LoopResult{}
	for _, check := range []func() LoopResult{
		ld.checkRepeat,
		ld.checkPingPong,
		ld.checkStall,
	} {
		r := check()
		if r.Severity > worst.Severity {
			worst = r
		}
	}
	return worst
}

// Reset clears all recorded entries.
func (ld *LoopDetector) Reset() {
	ld.mu.Lock()
	ld.entries = ld.entries[:0]
	ld.mu.Unlock()
}

// --- Detector 1: Generic Repeat (identical code hash streak) ---

func (ld *LoopDetector) checkRepeat() LoopResult {
	n := len(ld.entries)
	if n < 2 {
		return LoopResult{}
	}
	last := ld.entries[n-1].codeHash
	streak := 1
	for i := n - 2; i >= 0; i-- {
		if ld.entries[i].codeHash != last {
			break
		}
		streak++
	}
	return ld.classify(DetectorRepeat, streak, ld.cfg.RepeatWarning, ld.cfg.RepeatCritical,
		"identical tool call repeated")
}

// --- Detector 2: Ping-Pong (ABAB alternation) ---

func (ld *LoopDetector) checkPingPong() LoopResult {
	n := len(ld.entries)
	if n < 4 {
		return LoopResult{}
	}
	a := ld.entries[n-2].codeHash
	b := ld.entries[n-1].codeHash
	if a == b {
		return LoopResult{}
	}
	pairs := 1
	for i := n - 3; i >= 1; i -= 2 {
		if ld.entries[i].codeHash == b && ld.entries[i-1].codeHash == a {
			pairs++
		} else {
			break
		}
	}
	return ld.classify(DetectorPingPong, pairs, ld.cfg.PingPongWarning, ld.cfg.PingPongCritical,
		"alternating tool call pattern")
}

// --- Detector 3: No Progress (identical output streak) ---

func (ld *LoopDetector) checkStall() LoopResult {
	n := len(ld.entries)
	if n < 2 {
		return LoopResult{}
	}
	last := ld.entries[n-1].outputHash
	streak := 1
	for i := n - 2; i >= 0; i-- {
		if ld.entries[i].outputHash != last {
			break
		}
		streak++
	}
	return ld.classify(DetectorStall, streak, ld.cfg.StallWarning, ld.cfg.StallCritical,
		"identical output with no progress")
}

// classify maps a count to severity using warning/critical thresholds.
func (ld *LoopDetector) classify(kind DetectorKind, count, warn, crit int, base string) LoopResult {
	switch {
	case count >= crit:
		return LoopResult{Stuck: true, Severity: SeverityCritical, Detector: kind, Count: count,
			Message: base + " (critical)"}
	case count >= warn:
		return LoopResult{Stuck: true, Severity: SeverityWarning, Detector: kind, Count: count,
			Message: base + " (warning)"}
	default:
		return LoopResult{}
	}
}

func quickHash(s string) string {
	s = strings.TrimSpace(s)
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:8]) // 16-char hex, collision-safe for sliding window
}
