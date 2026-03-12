package tsgoapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreferredBinaryPrefersExistingCandidate(t *testing.T) {
	t.Setenv("PATH", "")

	repoRoot := t.TempDir()
	expected := filepath.Join(repoRoot, "poc", "tsgo-api", "bin", "tsgo-upstream")
	if err := os.MkdirAll(filepath.Dir(expected), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(expected, []byte(""), 0o755); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	if got := PreferredBinary(repoRoot); got != expected {
		t.Fatalf("PreferredBinary() = %q, want %q", got, expected)
	}
}

func TestPreferredBinaryPrefersRepoLocalInstall(t *testing.T) {
	t.Setenv("PATH", "")

	repoRoot := t.TempDir()
	expected := filepath.Join(repoRoot, ".tsgo", "node_modules", ".bin", "tsgo")
	if err := os.MkdirAll(filepath.Dir(expected), 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	if err := os.WriteFile(expected, []byte(""), 0o755); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	if got := PreferredBinary(repoRoot); got != expected {
		t.Fatalf("PreferredBinary() = %q, want %q", got, expected)
	}
}

func TestPreferredBinaryFallsBackToPath(t *testing.T) {
	repoRoot := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	executable := filepath.Join(binDir, "tsgo")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	t.Setenv("PATH", binDir)

	if got := PreferredBinary(repoRoot); got != executable {
		t.Fatalf("PreferredBinary() = %q, want %q", got, executable)
	}
}

func TestPreflightReportsMissingBinary(t *testing.T) {
	err := Preflight(filepath.Join(t.TempDir(), "missing-tsgo"), t.TempDir())
	if err == nil {
		t.Fatal("expected Preflight to fail for missing binary")
	}
	if !strings.Contains(err.Error(), "tsgo binary not found") {
		t.Fatalf("expected missing binary error, got %v", err)
	}
}

func TestPreflightUsesBinaryFromPath(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}

	executable := filepath.Join(binDir, "tsgo")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf 'Usage of api:'\n"), 0o755); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	t.Setenv("PATH", binDir)

	if err := Preflight("tsgo", t.TempDir()); err != nil {
		t.Fatalf("Preflight returned error: %v", err)
	}
}
