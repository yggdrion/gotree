package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPlanBatch(t *testing.T) {
	a := &Repo{Name: "a"}
	b := &Repo{Name: "b", Status: Status{Files: []string{"x"}}}
	c := &Repo{Name: "c"}
	d := &Repo{Name: "d", Status: Status{Files: []string{"y", "z"}}}
	clean, dirty := planBatch([]*Repo{a, b, c, d})
	if len(clean) != 2 || clean[0] != a || clean[1] != c {
		t.Errorf("clean = %v", clean)
	}
	if len(dirty) != 2 || dirty[0] != b || dirty[1] != d {
		t.Errorf("dirty = %v", dirty)
	}
}

func TestActionDropsVanishedTopLevel(t *testing.T) {
	m := newModel(t.TempDir())
	wt := &Repo{Path: t.TempDir(), Name: "wt", IsWorktree: true}
	keep := &Repo{Path: t.TempDir(), Name: "keep"}
	m.repos = []*Repo{wt, keep}
	m.busy[wt.Path] = true
	m.sel[wt.Path] = true
	m.update(actionDoneMsg{path: wt.Path, top: wt.Path, verb: "remove worktree"})
	if len(m.repos) != 1 || m.repos[0] != keep || m.busy[wt.Path] || m.sel[wt.Path] {
		t.Errorf("row not dropped: %v busy=%v sel=%v", m.repos, m.busy, m.sel)
	}
}

func dirtyRepo(n string) *Repo {
	return &Repo{Path: n, Name: n, Status: Status{Files: []string{"f"}, Default: "main"}}
}

func queued() (model, *Repo, *Repo, *Repo) {
	m := newModel("")
	c, d1, d2 := &Repo{Path: "clean", Name: "clean"}, dirtyRepo("d1"), dirtyRepo("d2")
	m.queue = []confirm{{d1, kBack, d1.Status.Files}, {d2, kBack, d2.Status.Files}}
	m.pending = []job{{kind: kBack, repo: c}}
	return m, c, d1, d2
}

func press(m *model, s string) {
	k := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	if s == "esc" {
		k = tea.KeyMsg{Type: tea.KeyEsc}
	}
	m.key(k)
}

func TestEscCancelsWholeBatch(t *testing.T) {
	m, c, d1, d2 := queued()
	press(&m, "y")
	press(&m, "esc")
	if len(m.queue) != 0 || len(m.pending) != 0 || m.busy[c.Path] || m.busy[d1.Path] || m.busy[d2.Path] {
		t.Errorf("queue=%v pending=%v busy=%v", m.queue, m.pending, m.busy)
	}
}

func TestOtherKeySkipsOnlyThatRepo(t *testing.T) {
	m, c, d1, d2 := queued()
	press(&m, "n")
	if m.busy[c.Path] {
		t.Fatal("started before queue empty")
	}
	press(&m, "y")
	if m.busy[d1.Path] || !m.busy[d2.Path] || !m.busy[c.Path] {
		t.Errorf("busy=%v", m.busy)
	}
}

func TestFetchErrorClearsButActionErrorSurvives(t *testing.T) {
	m := newModel("")
	m.errs["a"] = "fetch: boom\nline2"
	m.update(fetchedMsg{path: "a"})
	if _, ok := m.errs["a"]; ok {
		t.Errorf("stale fetch error kept: %v", m.errs)
	}
	m.update(fetchedMsg{path: "a", err: errors.New("fetch: again")})
	if m.errs["a"] != "fetch: again" {
		t.Errorf("errs = %v", m.errs)
	}
	m.errs["a"] = "pull: not fast-forward"
	m.update(fetchedMsg{path: "a"})
	m.update(fetchedMsg{path: "a", err: errors.New("fetch: x")})
	if m.errs["a"] != "pull: not fast-forward" {
		t.Errorf("action error lost: %v", m.errs)
	}
}

func TestCursorErrorFull(t *testing.T) {
	m := newModel("")
	m.repos = []*Repo{{Path: "a", Name: "a"}}
	m.cursor = "a"
	m.errs["a"] = "fetch: fatal: x\nhint: y"
	if got := m.cursorError(); got != "fetch: fatal: x · hint: y" {
		t.Errorf("got %q", got)
	}
	delete(m.errs, "a")
	m.repos[0].Status.Err = "bad repo"
	if got := m.cursorError(); got != "bad repo" {
		t.Errorf("got %q", got)
	}
}

func TestDiscardAbortsWhenFilesChangedSinceConfirm(t *testing.T) {
	_, clone := newFixture(t)
	os.WriteFile(filepath.Join(clone, "old.txt"), []byte("x"), 0o644)
	r := LoadRepo(clone)
	os.WriteFile(filepath.Join(clone, "new.txt"), []byte("precious"), 0o644)
	m := newModel("")
	m.repos = []*Repo{r}
	msg := m.run(job{kind: kBack, repo: r, discard: true, files: []string{"old.txt"}})().(actionDoneMsg)
	if msg.err == nil || !strings.Contains(msg.err.Error(), "changed since confirm") {
		t.Fatalf("err = %v", msg.err)
	}
	if _, err := os.Stat(filepath.Join(clone, "new.txt")); err != nil {
		t.Errorf("file deleted: %v", err)
	}
}

func TestStaleSelectionFallsBackToCursor(t *testing.T) {
	m := newModel("")
	keep := &Repo{Path: "keep", Name: "keep"}
	m.repos = []*Repo{keep}
	m.sel["gone"] = true
	if tg := m.targets(); len(tg) != 1 || tg[0] != keep {
		t.Errorf("targets = %v", tg)
	}
	wt := &Repo{Path: "wt", Name: "wt", IsWorktree: true}
	top := &Repo{Path: "top", Name: "top", Worktrees: []*Repo{wt}}
	m.repos = []*Repo{top}
	m.sel["wt"] = true
	m.replace(&Repo{Path: "top", Name: "top"})
	if m.sel["wt"] {
		t.Error("stale selection kept")
	}
}
