package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func releaseRepo(t *testing.T, failCheck string) func(...string) string {
	t.Helper()
	root := t.TempDir()
	for source, destination := range map[string]string{
		"release.sh": "scripts/release.sh", "../flake.nix": "flake.nix", "../qshare.spec": "qshare.spec",
	} {
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, destination)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	// Stub only Go checks; exercise real Git operations in an isolated repository.
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\n[ \"$1\" != \"$FAIL_CHECK\" ]\n"), 0755); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Release Tester", "GIT_AUTHOR_EMAIL=release@example.com",
		"GIT_COMMITTER_NAME=Release Tester", "GIT_COMMITTER_EMAIL=release@example.com",
		"FAIL_CHECK="+failCheck)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir, cmd.Env = root, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("git", "init", "-b", "main")
	run("git", "add", ".")
	run("git", "commit", "-m", "initial")
	return run
}

func TestReleaseSuccess(t *testing.T) {
	run := releaseRepo(t, "")
	_, oldChangelog, ok := strings.Cut(run("git", "show", "HEAD:qshare.spec"), "%changelog\n")
	if !ok {
		t.Fatal("fixture has no changelog")
	}
	run("bash", "scripts/release.sh", "0.7.0")
	if run("git", "cat-file", "-t", "v0.7.0") != "tag\n" ||
		run("git", "rev-parse", "v0.7.0^{commit}") != run("git", "rev-parse", "HEAD") {
		t.Fatal("expected annotated tag on release commit")
	}
	if run("git", "status", "--porcelain") != "" {
		t.Fatal("release left a dirty checkout")
	}
	flake := run("git", "show", "v0.7.0:flake.nix")
	spec := run("git", "show", "v0.7.0:qshare.spec")
	if !strings.Contains(flake, `packageVersion = "0.7.0";`) ||
		!strings.Contains(spec, "Version:        0.7.0\n") ||
		!strings.Contains(spec, "Release Tester <release@example.com> - 0.7.0-1\n- Release version 0.7.0\n\n"+oldChangelog) {
		t.Fatal("tagged package metadata is incorrect or previous changelog was changed")
	}
}

func TestReleasePreflight(t *testing.T) {
	for _, tc := range []struct{ name, setup, version, message string }{
		{"invalid", ":", "0.07.0", "expected a stable version"},
		{"duplicate", "git tag v0.7.0", "0.7.0", "tag v0.7.0 already exists"},
		{"dirty", "touch untracked", "0.7.0", "working tree and index must be clean"},
		{"layout", "truncate -s 0 flake.nix && git add flake.nix && git commit -m layout", "0.7.0", "expected exactly one packageVersion"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := releaseRepo(t, "")
			run("bash", "-c", tc.setup)
			before := run("git", "rev-parse", "HEAD")
			tags := run("git", "tag", "--list")
			status := run("git", "status", "--porcelain")
			out := run("bash", "-c", `! bash scripts/release.sh "$1"`, "release-test", tc.version)
			if !strings.Contains(out, tc.message) {
				t.Fatalf("expected %q: %s", tc.message, out)
			}
			if run("git", "rev-parse", "HEAD") != before || run("git", "tag", "--list") != tags ||
				run("git", "status", "--porcelain") != status {
				t.Fatal("preflight failure changed repository state")
			}
		})
	}
}

func TestReleaseValidationFailure(t *testing.T) {
	for _, check := range []string{"test", "vet"} {
		t.Run(check, func(t *testing.T) {
			run := releaseRepo(t, check)
			before := run("git", "rev-parse", "HEAD")
			run("bash", "-c", "! bash scripts/release.sh 0.7.0")
			if run("git", "rev-parse", "HEAD") != before || run("git", "tag", "--list") != "" {
				t.Fatal("validation failure created a commit or tag")
			}
			if run("git", "diff", "--name-only") != "flake.nix\nqshare.spec\n" {
				t.Fatal("expected retained package edits")
			}
		})
	}
}
