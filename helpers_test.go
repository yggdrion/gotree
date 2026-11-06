package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitT runs git in dir with the process env; returns trimmed stdout; fails the test with stderr+stdout on error.
func gitT(t *testing.T, dir string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v\n%s%s", strings.Join(args, " "), err, stderr.String(), stdout.String())
	}
	return strings.TrimSpace(stdout.String())
}

// isolateGit keeps the user's and system git config out of tests.
func isolateGit(t *testing.T) {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
}

// newFixture: bare remote (branch beta7, one commit) and a fresh clone of it.
func newFixture(t *testing.T) (remote, clone string) {
	t.Helper()
	isolateGit(t)
	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	seed := filepath.Join(root, "seed")
	clone = filepath.Join(root, "clone")
	gitT(t, root, "init", "--bare", "-b", "beta7", remote)
	gitT(t, root, "init", "-b", "beta7", seed)
	if err := os.WriteFile(filepath.Join(seed, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, seed, "add", ".")
	gitT(t, seed, "commit", "-m", "init")
	gitT(t, seed, "remote", "add", "origin", remote)
	gitT(t, seed, "push", "origin", "beta7")
	gitT(t, root, "clone", remote, clone)
	return remote, clone
}

// cloneOf makes an extra clone of remote (for pushing from "another machine").
func cloneOf(t *testing.T, remote string) string {
	t.Helper()
	isolateGit(t)
	dir := filepath.Join(t.TempDir(), "other")
	gitT(t, filepath.Dir(dir), "clone", remote, dir)
	return dir
}
