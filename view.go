package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	green, yellow, red, purple, grey = "42", "220", "196", "135", "245"
	cursorBG                         = "238"
)

const footer = " enter cd · c code · b back+pull · p pull · f fetch · r refresh · x select · →/← fold · d del worktree · / filter · ? help · q quit"

var helpLines = []string{
	"  ↑/↓ j/k   move",
	"  enter     cd into the folder and quit",
	"  c         open in VS Code",
	"  b         back to default branch + pull (selection or cursor row)",
	"  p         pull --ff-only (selection or cursor row)",
	"  f         fetch all repos",
	"  r         re-read local state of all repos and re-sort",
	"  x         select / unselect row",
	"  →/←       unfold / fold worktrees (← on a child jumps to parent)",
	"  d         remove worktree (asks first, branch is kept)",
	"  /         filter by name (enter keeps, esc clears)",
	"  ?         toggle this help",
	"  q / esc   quit",
}

func sty(fg string, cur bool) lipgloss.Style {
	s := lipgloss.NewStyle()
	if fg != "" {
		s = s.Foreground(lipgloss.Color(fg))
	}
	if cur {
		s = s.Background(lipgloss.Color(cursorBG))
	}
	return s
}

func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 3 {
		return string(r[:w])
	}
	return string(r[:w-3]) + "..."
}

func pad(s string, w int) string {
	if n := w - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

func (m model) width() int {
	if m.w <= 0 {
		return 100
	}
	return m.w
}

func (m model) dialogLines() []string {
	if len(m.queue) == 0 {
		return nil
	}
	c := m.queue[0]
	var l []string
	if c.kind == kRemove {
		l = append(l, fmt.Sprintf("Remove worktree %s? Branch %s is kept.", c.repo.Path, c.repo.Status.Branch))
		if len(c.files) > 0 {
			l = append(l, "Uncommitted changes, these changes will be lost:")
		}
	} else {
		l = append(l, fmt.Sprintf("Discard changes in %s and go back to %s? These changes will be lost:", c.repo.Name, c.repo.Status.Default))
	}
	for i, f := range c.files {
		if i == 10 {
			l = append(l, fmt.Sprintf("  +%d more", len(c.files)-10))
			break
		}
		l = append(l, "  "+f)
	}
	return append(l, "y = yes · esc = cancel all · any other key = skip this one")
}

// listH is the number of rows visible in the list area.
func (m model) listH() int {
	if m.h <= 0 {
		return 1000
	}
	return m.h - 4 - max(1, len(m.dialogLines()))
}

type cols struct{ name, branch, state, upd int }

func (m model) cols() cols {
	rest := m.width() - 3 - 7 - 6 // gutter, updated col, 3 gaps
	state := min(24, max(8, rest/4))
	rest -= state
	name := max(8, rest*2/5)
	return cols{name, max(8, rest-name), state, 7}
}

func stateColor(s Status) string {
	switch {
	case s.Err != "" || len(s.Files) > 0:
		return red
	case s.Ahead > 0 || s.Behind > 0:
		return yellow
	case s.Merged || s.Gone || s.Default == "" || s.Upstream == "":
		return grey
	}
	return green
}

func (m model) renderRow(rw row, cur bool, c cols, now time.Time) string {
	r := rw.Repo
	g0, g1 := " ", " "
	switch {
	case rw.Child:
		g0 = "└"
	case len(r.Worktrees) > 0 && r.Expanded:
		g0 = "▾"
	case len(r.Worktrees) > 0:
		g0 = "▸"
	}
	if m.sel[r.Path] {
		g1 = "✓"
	}
	badge := ""
	if !rw.Child && len(r.Worktrees) > 0 {
		badge = fmt.Sprintf(" ⎇%d", len(r.Worktrees))
	}
	nameFg := ""
	if rw.Child {
		nameFg = purple
	}
	n := trunc(r.Name, c.name-lipgloss.Width(badge))
	nameCell := sty(nameFg, cur).Render(n) + sty(purple, cur).Render(badge) +
		sty("", cur).Render(strings.Repeat(" ", max(0, c.name-lipgloss.Width(n)-lipgloss.Width(badge))))

	st, fg := stateText(r.Status), stateColor(r.Status)
	if e := m.errs[r.Path]; e != "" {
		st, fg = firstLineOf(e), red
	}
	if m.busy[r.Path] {
		st, fg = "…", yellow
	}
	gap := sty("", cur).Render("  ")
	return sty(purple, cur).Render(g0) + sty(green, cur).Render(g1) + sty("", cur).Render(" ") + nameCell + gap +
		sty("", cur).Render(pad(trunc(branchText(r.Status), c.branch), c.branch)) + gap +
		sty(fg, cur).Render(pad(trunc(st, c.state), c.state)) + gap +
		sty(grey, cur).Render(pad(relTime(now, r.Status.Updated), c.upd))
}

func (m model) header() string {
	root := m.root
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(home, root); err == nil && !strings.HasPrefix(rel, "..") {
			root = filepath.Join("~", rel)
		}
	}
	l := " gotree  " + root
	if v := m.filter.Value(); v != "" {
		l += "  /" + v
	}
	r := fmt.Sprintf("%d repos", len(m.repos))
	switch {
	case m.loading:
		r += "   loading…"
	case m.fetching():
		r += fmt.Sprintf("   fetching… %d/%d", m.fetchDone, m.fetchTotal)
	}
	gap := max(1, m.width()-lipgloss.Width(l)-lipgloss.Width(r)-1)
	return lipgloss.NewStyle().Bold(true).Render(l) + strings.Repeat(" ", gap) + sty(grey, false).Render(r)
}

func (m model) View() string {
	w := m.width()
	c := m.cols()
	var b []string
	b = append(b, m.header())
	b = append(b, sty(grey, false).Bold(true).Render(pad("   "+pad("REPO", c.name)+"  "+pad("BRANCH", c.branch)+"  "+pad("STATE", c.state)+"  UPDATED", w)))
	var body []string
	if m.help {
		body = helpLines
	} else {
		now := time.Now()
		rs := m.rows()
		for i := m.off; i < len(rs) && i < m.off+m.listH(); i++ {
			body = append(body, m.renderRow(rs[i], i == m.idx, c, now))
		}
	}
	for len(body) < m.listH() && m.h > 0 {
		body = append(body, "")
	}
	b = append(b, body...)
	b = append(b, sty(grey, false).Render(strings.Repeat("─", w)))
	switch d := m.dialogLines(); {
	case len(d) > 0:
		for _, l := range d {
			b = append(b, sty(yellow, false).Render(trunc(" "+l, w)))
		}
	case m.filterOn:
		b = append(b, " "+m.filter.View())
	case m.status == "" && m.cursorError() != "":
		b = append(b, sty(red, false).Render(trunc(" "+m.cursorError(), w)))
	case m.status != "":
		fg := red
		if m.statusOK {
			fg = green
		}
		b = append(b, sty(fg, false).Render(trunc(" "+m.status, w)))
	default:
		b = append(b, "")
	}
	b = append(b, sty(grey, false).Render(trunc(footer, w)))
	return strings.Join(b, "\n")
}
