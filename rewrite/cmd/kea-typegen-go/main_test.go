package main

import (
	"path/filepath"
	"testing"
)

func TestCLIWorkingDirUsesInvocationEnv(t *testing.T) {
	t.Setenv(invocationDirEnv, filepath.Join(string(filepath.Separator), "tmp", "kea-typegen-invocation"))

	if got := cliWorkingDir(); got != filepath.Join(string(filepath.Separator), "tmp", "kea-typegen-invocation") {
		t.Fatalf("cliWorkingDir() = %q", got)
	}
}

func TestMustAbsFromResolvesRelativePathsAgainstBase(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "tmp", "kea-typegen-base")
	got := mustAbsFrom(base, filepath.Join("samples", "logic.ts"))
	want := filepath.Join(base, "samples", "logic.ts")
	if got != want {
		t.Fatalf("mustAbsFrom() = %q, want %q", got, want)
	}
}
