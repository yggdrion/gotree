package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/yggdrion/gotree/internal/repo"
)

type menuItem struct {
	key, label string
	why        string // non-empty = disabled, shows reason
	run        func() tea.Cmd
}

type menuState struct {
	r     *repo.Repo
	items []menuItem
	sel   int
}

func (m *model) cd(r *repo.Repo) tea.Cmd {
	m.chosen = r.Path
	return tea.Quit
}

func (m *model) openEditor(r *repo.Repo) {
	if err := repo.OpenEditor(r.Path); err != nil {
		m.setStatus(false, "%s", err)
	} else {
		m.setStatus(true, "opened %s in VS Code", r.Name)
	}
}

func (m *model) toggleSel(r *repo.Repo) {
	if m.sel[r.Path] {
		delete(m.sel, r.Path)
	} else {
		m.sel[r.Path] = true
	}
}

func (m *model) removeWT(r *repo.Repo) {
	switch {
	case !r.IsWorktree:
		m.setStatus(false, "d: only on worktree rows")
	case m.busy[r.Path] || m.busy[m.topPath(r.Path)]:
		m.setStatus(false, "busy")
	default:
		m.queue = append(m.queue, confirm{r, kRemove, r.Status.Files})
	}
}

// why returns a reason an action cannot run on r, "" if fine.
func whyNot(k kind, r *repo.Repo) string {
	switch {
	case k == kBack && r.IsWorktree:
		return "not allowed on a worktree"
	case k == kBack && r.Status.Default == "":
		return "unknown default branch"
	case k == kPull && r.Status.Upstream == "":
		return "no upstream"
	case k == kPull && r.Status.Gone:
		return "upstream gone"
	}
	return ""
}

func (m *model) openMenu(r *repo.Repo) {
	items := []menuItem{
		{key: "x", label: "cd into folder", run: func() tea.Cmd { return m.cd(r) }},
		{key: "c", label: "open in VS Code", run: func() tea.Cmd { m.openEditor(r); return nil }},
		{key: "b", label: "back to default branch + pull", why: whyNot(kBack, r), run: func() tea.Cmd { return m.dispatch(kBack) }},
		{key: "p", label: "pull --ff-only", why: whyNot(kPull, r), run: func() tea.Cmd { return m.dispatch(kPull) }},
		{key: "space", label: "select / unselect", run: func() tea.Cmd { m.toggleSel(r); return nil }},
	}
	if r.IsWorktree {
		items = append(items, menuItem{key: "d", label: "remove worktree", run: func() tea.Cmd { m.removeWT(r); return nil }})
	}
	m.menu = &menuState{r: r, items: items}
}

func (m *model) menuKey(s string) tea.Cmd {
	mn := m.menu
	run := func(it menuItem) tea.Cmd {
		if it.why != "" {
			m.setStatus(false, "%s: %s", it.label, it.why)
			return nil
		}
		m.menu = nil
		return it.run()
	}
	switch s {
	case "esc", "q":
		m.menu = nil
	case "up", "k":
		mn.sel = max(0, mn.sel-1)
	case "down", "j":
		mn.sel = min(len(mn.items)-1, mn.sel+1)
	case "enter":
		return run(mn.items[mn.sel])
	default:
		for _, it := range mn.items {
			if it.key == s {
				return run(it)
			}
		}
	}
	return nil
}

// menuLines renders the overlay: repo state summary, then the actions.
func (m model) menuLines() []string {
	mn := m.menu
	r, st := mn.r, mn.r.Status
	l := []string{
		" " + r.Name + "  " + r.Path,
		"   branch  " + branchText(st) + "   default " + orDash(st.Default),
		"   state   " + stateText(st),
		fmt.Sprintf("   remote  upstream %s · ↑%d ↓%d · %d uncommitted", orDash(st.Upstream), st.Ahead, st.Behind, len(st.Files)),
	}
	if e := m.errs[r.Path]; e != "" {
		l = append(l, "   error   "+repo.FirstLine(e))
	}
	l = append(l, "")
	for i, it := range mn.items {
		c := "  "
		if i == mn.sel {
			c = "▸ "
		}
		line := fmt.Sprintf(" %s%-6s %s", c, it.key, it.label)
		if it.why != "" {
			line += "  (" + it.why + ")"
		}
		l = append(l, line)
	}
	return append(l, "", " enter/key run · ↑/↓ move · esc close")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
