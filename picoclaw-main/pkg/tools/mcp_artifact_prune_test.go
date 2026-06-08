package tools

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneExpiredArtifacts(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	fresh := filepath.Join(dir, "fresh.txt")
	stale := filepath.Join(dir, "stale.txt")
	for _, p := range []string{fresh, stale} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	// Age the stale file beyond the TTL.
	old := now.Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	pruneExpiredArtifacts(dir, 24*time.Hour, now)

	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh artifact should be kept, got %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale artifact should be removed, stat err = %v", err)
	}
}

func TestPruneExpiredArtifacts_MissingDirIsNoOp(t *testing.T) {
	// Must not panic or error on a non-existent directory.
	pruneExpiredArtifacts(filepath.Join(t.TempDir(), "does-not-exist"), time.Hour, time.Now())
}

func TestPruneExpiredArtifacts_ZeroTTLDisabled(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	old := time.Now().Add(-1000 * time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	pruneExpiredArtifacts(dir, 0, time.Now())
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("ttl<=0 must disable pruning, got %v", err)
	}
}
