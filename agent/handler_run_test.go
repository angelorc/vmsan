package main

import (
	"testing"
)

func TestBuildCommand_WrapsBareCommandsWithEnv(t *testing.T) {
	envBinary := findEnvBinary()
	if envBinary == "" {
		t.Fatal("expected env binary to exist on test host")
	}

	cmd := buildCommand("openclaw", []string{"--version"})
	if cmd.Path != envBinary {
		t.Fatalf("expected Path=%q, got %q", envBinary, cmd.Path)
	}

	wantArgs := []string{envBinary, "--", "openclaw", "--version"}
	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(wantArgs), len(cmd.Args), cmd.Args)
	}
	for i, want := range wantArgs {
		if cmd.Args[i] != want {
			t.Fatalf("expected arg %d to be %q, got %q", i, want, cmd.Args[i])
		}
	}
}

func TestBuildCommand_PreservesExplicitPath(t *testing.T) {
	cmd := buildCommand("/usr/local/bin/npm", []string{"--version"})
	if cmd.Path != "/usr/local/bin/npm" {
		t.Fatalf("expected Path to stay explicit, got %q", cmd.Path)
	}
	wantArgs := []string{"/usr/local/bin/npm", "--version"}
	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(wantArgs), len(cmd.Args), cmd.Args)
	}
	for i, want := range wantArgs {
		if cmd.Args[i] != want {
			t.Fatalf("expected arg %d to be %q, got %q", i, want, cmd.Args[i])
		}
	}
}
