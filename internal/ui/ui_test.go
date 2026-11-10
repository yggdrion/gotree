package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/yggdrion/gotree/internal/repo"
)

func TestPlanBatch(t *testing.T) {
	a := &repo.Repo{Name: "a"}
	b := &repo.Repo{Name: "b", Status: repo.Status{Files: []string{"x"}}}
	c := &repo.Repo{Name: "c"}
	d := &repo.Repo{Name: "d", Status: repo.Status{Files: []string{"y", "z"}}}
	clean, dirty := planBatch([]*repo.Repo{a, b, c, d})
	if len(clean) != 2 || clean[0] != a || clean[1] != c {
		t.Errorf("clean = %v", clean)
	}
	if len(dirty) != 2 || dirty[0] != b || dirty[1] != d {
		t.Errorf("dirty = %v", dirty)
	}
}

func TestActionDropsVanishedTopLevel(t *testing.T) {
	m := newModel(t.TempDir())
	wt := &repo.Repo{Path: t.TempDir(), Name: "wt", IsWorktree: true}
	keep := &repo.Repo{Path: t.TempDir(), Name: "keep"}
	m.repos = []*repo.Repo{wt, keep}
	m.busy[wt.Path] = true
	m.sel[wt.Path] = true
	m.update(actionDoneMsg{path: wt.Path, top: wt.Path, verb: "remove worktree"})
	if len(m.repos) != 1 || m.repos[0] != keep || m.busy[wt.Path] || m.sel[wt.Path] {
		t.Errorf("row not dropped: %v busy=%v sel=%v", m.repos, m.busy, m.sel)
	}
}

func dirtyRepo(n string) *repo.Repo {
	return &repo.Repo{Path: n, Name: n, Status: repo.Status{Files: []string{"f"}, Default: "main"}}
}

func queued() (model, *repo.Repo, *repo.Repo, *repo.Repo) {
	m := newModel("")
	c, d1, d2 := &repo.Repo{Path: "clean", Name: "clean"}, dirtyRepo("d1"), dirtyRepo("d2")
	m.queue = []confirm{{d1, kBack, d1.Status.Files}, {d2, kBack, d2.Status.Files}}
	m.pending = []job{{kind: kBack, repo: c}}
	return m, c, d1, d2
}

func press(m *model, s string) {
	m.key(key(s))
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
	m.repos = []*repo.Repo{{Path: "a", Name: "a"}}
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
	mustWrite(t, filepath.Join(clone, "old.txt"), "x")
	r := repo.LoadRepo(clone)
	mustWrite(t, filepath.Join(clone, "new.txt"), "precious")
	m := newModel("")
	m.repos = []*repo.Repo{r}
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
	keep := &repo.Repo{Path: "keep", Name: "keep"}
	m.repos = []*repo.Repo{keep}
	m.sel["gone"] = true
	if tg := m.targets(); len(tg) != 1 || tg[0] != keep {
		t.Errorf("targets = %v", tg)
	}
	wt := &repo.Repo{Path: "wt", Name: "wt", IsWorktree: true}
	top := &repo.Repo{Path: "top", Name: "top", Worktrees: []*repo.Repo{wt}}
	m.repos = []*repo.Repo{top}
	m.sel["wt"] = true
	m.replace(&repo.Repo{Path: "top", Name: "top"})
	if m.sel["wt"] {
		t.Error("stale selection kept")
	}
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space", " ":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	r := []rune(s)
	return tea.KeyPressMsg{Code: r[0], Text: s}
}

func TestEnterMenu(t *testing.T) {
	m := newModel("")
	r := &repo.Repo{Path: "r", Name: "r", Status: repo.Status{Branch: "main", Default: "main"}}
	m.repos = []*repo.Repo{r}
	m.fix()
	m.key(key("enter"))
	if m.menu == nil || m.chosen != "" {
		t.Fatal("enter should open menu without choosing")
	}
	if !strings.Contains(strings.Join(m.menuLines(), "\n"), "branch") {
		t.Error("menu lacks repo state")
	}
	m.key(key("esc"))
	if m.menu != nil {
		t.Fatal("esc should close menu")
	}
	m.key(key(" "))
	if !m.sel["r"] {
		t.Error("space should select")
	}
	m.key(key("enter"))
	m.key(key("enter")) // first item = cd
	if m.chosen != "r" {
		t.Errorf("chosen = %q", m.chosen)
	}
	m = newModel("")
	m.repos = []*repo.Repo{r}
	m.fix()
	m.key(key("x"))
	if m.chosen != "r" {
		t.Error("x should cd")
	}
}

func TestShiftRKeyOpensReset(t *testing.T) {
	m := newModel(t.TempDir())
	r := &repo.Repo{Name: "x", Status: repo.Status{Upstream: "origin/main"}}
	m.openMenu(r)
	m.key(tea.KeyPressMsg{Code: 'r', ShiftedCode: 'R', Mod: tea.ModShift})
	if len(m.queue) != 1 || m.queue[0].kind != kReset {
		t.Fatalf("queue = %v", m.queue)
	}
}

func TestSortToggleKeepsCursor(t *testing.T) {
	m := newModel(t.TempDir())
	old := &repo.Repo{Path: "/x/b", Name: "b"}
	newer := &repo.Repo{Path: "/x/a", Name: "z", Status: repo.Status{Updated: time.Unix(100, 0)}}
	m.repos = []*repo.Repo{newer, old}
	m.cursor = old.Path
	m.update(key("s"))
	if m.sort != repo.SortName || m.repos[0] != old || m.cursor != old.Path {
		t.Fatalf("sort=%v repos=%v cursor=%s", m.sort, m.repos, m.cursor)
	}
	m.update(key("s"))
	if m.sort != repo.SortUpdated || m.repos[0] != newer {
		t.Fatalf("not back to updated: %v", m.repos)
	}
}

func TestShiftRKeyOnListOpensReset(t *testing.T) {
	m := newModel(t.TempDir())
	m.repos = []*repo.Repo{{Path: "/x/a", Name: "a", Status: repo.Status{Upstream: "origin/main"}}}
	m.fix()
	m.key(tea.KeyPressMsg{Code: 'r', ShiftedCode: 'R', Mod: tea.ModShift})
	if len(m.queue) != 1 || m.queue[0].kind != kReset {
		t.Fatalf("queue = %v", m.queue)
	}
	m = newModel(t.TempDir())
	m.repos = []*repo.Repo{{Path: "/x/a", Name: "a"}}
	m.fix()
	m.key(key("R"))
	if len(m.queue) != 0 || !strings.Contains(m.status, "no upstream") {
		t.Fatalf("queue=%v status=%q", m.queue, m.status)
	}
}

func TestDiffOverlay(t *testing.T) {
	_, clone := newFixture(t)
	m := newModel(filepath.Dir(clone))
	m.w, m.h = 80, 20
	m.repos = []*repo.Repo{repo.LoadRepo(clone)}
	m.fix()
	m.key(key("v"))
	if m.diff != nil || !strings.Contains(m.status, "no changes") {
		t.Fatalf("clean repo: diff=%v status=%q", m.diff, m.status)
	}
	mustWrite(t, filepath.Join(clone, "a.txt"), "changed\n")
	mustWrite(t, filepath.Join(clone, "new.txt"), "n\n")
	m.key(key("v"))
	if m.diff == nil {
		t.Fatalf("no diff opened, status=%q", m.status)
	}
	out := m.render()
	for _, want := range []string{"+changed", "-a", "? new.txt", "esc close"} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q", want)
		}
	}
	m.key(key("G"))
	if m.diff.off < 0 || m.diff.off > len(m.diff.lines) {
		t.Errorf("off = %d", m.diff.off)
	}
	m.key(key("esc"))
	if m.diff != nil {
		t.Error("esc should close diff")
	}
}

// Update has a value receiver, so menu actions must act on the model passed
// to the next Update, not on the copy that opened the menu.
func TestMenuCdThroughUpdate(t *testing.T) {
	var tm tea.Model = newModel(t.TempDir())
	m := tm.(model)
	m.repos = []*repo.Repo{{Path: "/x/a", Name: "a"}}
	m.fix()
	tm, _ = m.Update(key("enter"))
	tm, cmd := tm.Update(key("x"))
	if got := tm.(model).Chosen(); got != "/x/a" {
		t.Fatalf("chosen = %q", got)
	}
	if cmd == nil {
		t.Fatal("want quit cmd")
	}
}

func TestPullAllOnlySimple(t *testing.T) {
	m := newModel(t.TempDir())
	up := repo.Status{Upstream: "origin/main", Behind: 2}
	ok := &repo.Repo{Path: "/x/ok", Name: "ok", Status: up}
	wt := &repo.Repo{Path: "/x/wt", Name: "wt", IsWorktree: true, Status: up}
	ok.Worktrees = []*repo.Repo{wt}
	ahead := up
	ahead.Ahead = 1
	dirty := up
	dirty.Files = []string{"f"}
	gone := up
	gone.Gone = true
	current := repo.Status{Upstream: "origin/main"}
	m.repos = []*repo.Repo{
		ok,
		{Path: "/x/ahead", Name: "ahead", Status: ahead},
		{Path: "/x/dirty", Name: "dirty", Status: dirty},
		{Path: "/x/gone", Name: "gone", Status: gone},
		{Path: "/x/current", Name: "current", Status: current},
		{Path: "/x/noup", Name: "noup", Status: repo.Status{Behind: 1}},
	}
	m.fix()
	m.key(tea.KeyPressMsg{Code: 'p', ShiftedCode: 'P', Mod: tea.ModShift})
	t.Cleanup(func() { inflight.Delete(ok.Path); inflight.Delete(wt.Path) })
	if len(m.busy) != 2 || !m.busy[ok.Path] || !m.busy[wt.Path] {
		t.Fatalf("busy = %v", m.busy)
	}

	m = newModel(t.TempDir())
	m.repos = []*repo.Repo{{Path: "/x/current", Name: "current", Status: current}}
	m.fix()
	m.key(key("P"))
	if len(m.busy) != 0 || !strings.Contains(m.status, "nothing") {
		t.Fatalf("busy=%v status=%q", m.busy, m.status)
	}
}

// esc/q close an open overlay first; only the next press quits.
func TestEscClosesOverlayBeforeQuit(t *testing.T) {
	_, clone := newFixture(t)
	m := newModel(filepath.Dir(clone))
	m.w, m.h = 100, 30
	m.repos = []*repo.Repo{repo.LoadRepo(clone)}
	m.fix()
	for _, k := range []string{"esc", "q"} {
		m.key(key("?"))
		if !m.help {
			t.Fatal("? should open help")
		}
		out := m.render()
		if !strings.Contains(out, "show this help") || !strings.Contains(out, "clone") {
			t.Errorf("help should overlay the repo list:\n%s", out)
		}
		if c := m.key(key(k)); c != nil || m.help {
			t.Fatalf("%s should close help without quitting", k)
		}
		m.key(key("enter"))
		if m.menu == nil {
			t.Fatal("enter should open menu")
		}
		if c := m.key(key(k)); c != nil || m.menu != nil {
			t.Fatalf("%s should close menu without quitting", k)
		}
		if c := m.key(key(k)); c == nil {
			t.Fatalf("%s with no overlay should quit", k)
		}
	}
}
