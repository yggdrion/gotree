package repo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestQuickOrder(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a", "b", "c"} {
		mustMkdir(t, filepath.Join(root, d))
	}
	mustMkdir(t, filepath.Join(root, "a", ".git"))
	mustMkdir(t, filepath.Join(root, "b", ".git"))
	old, newer := time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour)
	for _, p := range []string{"a", "a/.git", "b", "b/.git", "c"} {
		mustChtimes(t, filepath.Join(root, p), old)
	}
	mustChtimes(t, filepath.Join(root, "b"), newer)
	got := QuickOrder(root)
	want := []string{filepath.Join(root, "b"), filepath.Join(root, "a")}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestLoadRepoNotRepo(t *testing.T) {
	isolateGit(t)
	if r := LoadRepo(t.TempDir()); r != nil {
		t.Fatalf("want nil, got %+v", r)
	}
}

func TestLoadRepoWorktrees(t *testing.T) {
	_, clone := newFixture(t)
	wt := filepath.Join(clone, ".claude", "worktrees", "wt1")
	gitT(t, clone, "worktree", "add", wt, "-b", "claude/wt1")
	r := LoadRepo(clone)
	if r == nil || r.IsWorktree || len(r.Worktrees) != 1 {
		t.Fatalf("unexpected: %+v", r)
	}
	w := r.Worktrees[0]
	if w.Status.Branch != "claude/wt1" || !w.IsWorktree || !SamePath(w.MainPath, clone) || w.Name != "wt1" || w.CommonDir != r.CommonDir {
		t.Fatalf("unexpected worktree: %+v", w)
	}
}

func TestLoadRepoSkipsMissingWorktree(t *testing.T) {
	_, clone := newFixture(t)
	wt := filepath.Join(clone, ".claude", "worktrees", "wt1")
	gitT(t, clone, "worktree", "add", wt, "-b", "claude/wt1")
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	r := LoadRepo(clone)
	if r == nil || len(r.Worktrees) != 0 {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestDedupeLinkedWorktree(t *testing.T) {
	_, clone := newFixture(t)
	side := filepath.Join(t.TempDir(), "side")
	gitT(t, clone, "worktree", "add", side)
	main, linked := LoadRepo(clone), LoadRepo(side)
	if main == nil || linked == nil || !linked.IsWorktree || !SamePath(linked.MainPath, clone) {
		t.Fatalf("load: %+v %+v", main, linked)
	}
	got := Dedupe([]*Repo{linked, main})
	if len(got) != 1 || got[0] != main {
		t.Fatalf("unexpected: %+v", got)
	}
	// orphan worktree stays when its main isn't listed
	if got := Dedupe([]*Repo{linked}); len(got) != 1 {
		t.Fatalf("orphan dropped: %+v", got)
	}
}

func TestSortRepos(t *testing.T) {
	t1 := time.Unix(1000, 0)
	mk := func(n string, d int) *Repo {
		return &Repo{Name: n, Status: Status{Updated: t1.Add(time.Duration(d) * time.Hour)}}
	}
	rs := []*Repo{mk("1", 0), mk("3", 2), mk("2", 1)}
	SortRepos(rs)
	if rs[0].Name != "3" || rs[1].Name != "2" || rs[2].Name != "1" {
		t.Fatalf("unexpected order")
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustChtimes(t *testing.T, p string, ts time.Time) {
	t.Helper()
	if err := os.Chtimes(p, ts, ts); err != nil {
		t.Fatal(err)
	}
}

func TestDedupeCaseInsensitiveCommonDir(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive paths are Windows-only")
	}
	_, clone := newFixture(t)
	side := filepath.Join(t.TempDir(), "side")
	gitT(t, clone, "worktree", "add", side)
	main, linked := LoadRepo(clone), LoadRepo(side)
	linked.CommonDir = strings.ToUpper(linked.CommonDir)
	if got := Dedupe([]*Repo{linked, main}); len(got) != 1 || got[0] != main {
		t.Fatalf("unexpected: %+v", got)
	}
}
