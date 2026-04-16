package recursion

import "testing"

func TestNormalizeGateMode(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected GateMode
	}{
		{name: "empty defaults to manual", raw: "", expected: GateModeManual},
		{name: "manual preserved", raw: "manual", expected: GateModeManual},
		{name: "auto aliases to manual", raw: "auto", expected: GateModeManual},
		{name: "force preserved", raw: "force", expected: GateModeForce},
		{name: "invalid falls back to manual", raw: "unexpected", expected: GateModeManual},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeGateMode(tt.raw); got != tt.expected {
				t.Fatalf("normalizeGateMode(%q) = %q, want %q", tt.raw, got, tt.expected)
			}
		})
	}
}
