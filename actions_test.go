package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wantErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

func TestBackToDefault(t *testing.T) {
	remote, clone := newFixture(t)
	gitT(t, clone, "checkout", "-b", "feat")
	other := cloneOf(t, remote)
	write(t, filepath.Join(other, "b.txt"), "b\n")
	gitT(t, other, "add", ".")
	gitT(t, other, "commit", "-m", "more")
	gitT(t, other, "push", "origin", "beta7")
	if err := BackToDefault(LoadRepo(clone), false); err != nil {
		t.Fatal(err)
	}
	s := ReadStatus(clone)
	if s.Branch != "beta7" || s.Behind != 0 {
		t.Fatalf("status = %+v", s)
	}
	if h, r := gitT(t, clone, "rev-parse", "HEAD"), gitT(t, remote, "rev-parse", "beta7"); h != r {
		t.Fatalf("HEAD %s != remote %s", h, r)
	}
}

func TestBackToDefaultDiscard(t *testing.T) {
	_, clone := newFixture(t)
	write(t, filepath.Join(clone, ".gitignore"), "*.log\n")
	gitT(t, clone, "add", ".")
	gitT(t, clone, "commit", "-m", "ignore")
	gitT(t, clone, "push", "origin", "beta7")
	write(t, filepath.Join(clone, "a.txt"), "changed\n")
	write(t, filepath.Join(clone, "new.txt"), "n\n")
	write(t, filepath.Join(clone, "x.log"), "l\n")
	if err := BackToDefault(LoadRepo(clone), true); err != nil {
		t.Fatal(err)
	}
	if s := ReadStatus(clone); len(s.Files) != 0 && !(len(s.Files) == 1 && s.Files[0] == "x.log") {
		t.Fatalf("files = %v", s.Files)
	}
	if _, err := os.Stat(filepath.Join(clone, "x.log")); err != nil {
		t.Fatal("ignored file removed")
	}
	if _, err := os.Stat(filepath.Join(clone, "new.txt")); err == nil {
		t.Fatal("untracked file kept")
	}
}

func TestBackToDefaultDirtyNoDiscard(t *testing.T) {
	_, clone := newFixture(t)
	gitT(t, clone, "checkout", "-b", "feat")
	write(t, filepath.Join(clone, "a.txt"), "feat\n")
	gitT(t, clone, "commit", "-am", "feat")
	write(t, filepath.Join(clone, "a.txt"), "dirty\n")
	err := BackToDefault(LoadRepo(clone), false)
	if err == nil || len(err.Error()) < 10 || err.Error()[:10] != "checkout: " {
		t.Fatalf("err = %v", err)
	}
}

func TestBackToDefaultRefusals(t *testing.T) {
	wantErr(t, BackToDefault(&Repo{IsWorktree: true}, false), "not allowed on a worktree")
	wantErr(t, BackToDefault(&Repo{}, false), "unknown default branch")
}

func TestPullNotFastForward(t *testing.T) {
	remote, clone := newFixture(t)
	write(t, filepath.Join(clone, "l.txt"), "l\n")
	gitT(t, clone, "add", ".")
	gitT(t, clone, "commit", "-m", "local")
	other := cloneOf(t, remote)
	write(t, filepath.Join(other, "r.txt"), "r\n")
	gitT(t, other, "add", ".")
	gitT(t, other, "commit", "-m", "remote")
	gitT(t, other, "push", "origin", "beta7")
	gitT(t, clone, "fetch")
	wantErr(t, Pull(LoadRepo(clone)), "pull: not fast-forward")
}

func TestPullRefusals(t *testing.T) {
	wantErr(t, Pull(&Repo{}), "no upstream")
	wantErr(t, Pull(&Repo{Status: Status{Upstream: "origin/x", Gone: true}}), "upstream gone")
}

func TestFetchSetsHead(t *testing.T) {
	_, clone := newFixture(t)
	gitT(t, clone, "remote", "set-head", "origin", "-d")
	if err := Fetch(clone); err != nil {
		t.Fatal(err)
	}
	if d := ReadStatus(clone).Default; d != "beta7" {
		t.Fatalf("default = %q", d)
	}
}

func TestOpenEditorMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	wantErr(t, OpenEditor("."), "VS Code 'code' command not found on PATH")
}

func TestRemoveWorktreeKeepsBranch(t *testing.T) {
	_, clone := newFixture(t)
	wt := filepath.Join(t.TempDir(), "wt1")
	gitT(t, clone, "worktree", "add", "-b", "claude/wt1", wt)
	main := LoadRepo(clone)
	if len(main.Worktrees) != 1 {
		t.Fatalf("worktrees = %d", len(main.Worktrees))
	}
	if err := RemoveWorktree(main.Worktrees[0], false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); err == nil {
		t.Fatal("worktree dir remains")
	}
	if gitT(t, clone, "branch", "--list", "claude/wt1") == "" {
		t.Fatal("branch deleted")
	}
}

func TestFetchTimesOut(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("no ssh")
	}
	_, clone := newFixture(t)
	old := netTimeout
	netTimeout = time.Second
	defer func() { netTimeout = old }()
	gitT(t, clone, "remote", "set-url", "origin", "ssh://git@10.255.255.1/x.git")
	start := time.Now()
	err := Fetch(clone)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("err=%v after %v", err, time.Since(start))
	}
}
