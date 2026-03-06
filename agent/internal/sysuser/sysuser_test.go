package sysuser

import (
	"os/exec"
	"testing"
)

func envMap(t *testing.T, env []string) map[string]string {
	t.Helper()

	out := make(map[string]string, len(env))
	for _, entry := range env {
		for i := 0; i < len(entry); i++ {
			if entry[i] != '=' {
				continue
			}
			key := entry[:i]
			if _, exists := out[key]; exists {
				t.Fatalf("duplicate environment entry for %s", key)
			}
			out[key] = entry[i+1:]
			break
		}
	}

	return out
}

func TestCredentialsApply_SetsCanonicalUserEnvAndPath(t *testing.T) {
	creds := &Credentials{
		Uid:      1000,
		Gid:      1000,
		HomeDir:  "/home/ubuntu",
		Username: "ubuntu",
	}

	cmd := exec.Command("sh")
	cmd.Env = []string{
		"PATH=/custom/bin:/usr/bin:/bin",
		"HOME=/root",
		"USER=root",
		"LOGNAME=root",
		"LANG=C.UTF-8",
		"TERM=xterm-256color",
	}

	creds.Apply(cmd)

	if cmd.Dir != creds.HomeDir {
		t.Fatalf("expected cmd.Dir=%q, got %q", creds.HomeDir, cmd.Dir)
	}

	env := envMap(t, cmd.Env)
	if env["HOME"] != creds.HomeDir {
		t.Fatalf("expected HOME=%q, got %q", creds.HomeDir, env["HOME"])
	}
	if env["USER"] != creds.Username {
		t.Fatalf("expected USER=%q, got %q", creds.Username, env["USER"])
	}
	if env["LOGNAME"] != creds.Username {
		t.Fatalf("expected LOGNAME=%q, got %q", creds.Username, env["LOGNAME"])
	}
	if env["TERM"] != "xterm-256color" {
		t.Fatalf("expected TERM to be preserved, got %q", env["TERM"])
	}
	if env["LANG"] != "C.UTF-8" {
		t.Fatalf("expected LANG to be preserved, got %q", env["LANG"])
	}

	wantPath := "/home/ubuntu/.npm-global/bin:/home/ubuntu/.local/bin:/custom/bin:/usr/bin:/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/sbin"
	if env["PATH"] != wantPath {
		t.Fatalf("expected PATH=%q, got %q", wantPath, env["PATH"])
	}
}

func TestCredentialsApply_PreservesExplicitDirAndUsesLastPathValue(t *testing.T) {
	creds := &Credentials{
		Uid:      1000,
		Gid:      1000,
		HomeDir:  "/home/ubuntu",
		Username: "ubuntu",
	}

	cmd := exec.Command("sh")
	cmd.Dir = "/workspace"
	cmd.Env = []string{
		"PATH=/usr/bin:/bin",
		"PATH=/team/bin:/usr/bin",
	}

	creds.Apply(cmd)

	if cmd.Dir != "/workspace" {
		t.Fatalf("expected cmd.Dir to stay /workspace, got %q", cmd.Dir)
	}

	env := envMap(t, cmd.Env)
	wantPath := "/home/ubuntu/.npm-global/bin:/home/ubuntu/.local/bin:/team/bin:/usr/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/sbin:/bin"
	if env["PATH"] != wantPath {
		t.Fatalf("expected PATH=%q, got %q", wantPath, env["PATH"])
	}
}
