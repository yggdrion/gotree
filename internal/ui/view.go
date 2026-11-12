package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yggdrion/gotree/internal/repo"
)

const (
	green, yellow, red, purple, grey = "42", "220", "196", "135", "245"
	cursorBG, selectedBG             = "238", "24"
)

const footer = " enter menu · x cd · c code · b back+pull · p pull · P pull all · R reset · v diff · space select · f fetch · r refresh · s sort · →/← fold · d del worktree · / filter · ? help · q quit"

var helpLines = []string{
	"  ↑/↓ j/k   move",
	"  enter     action menu (shows repo state)",
	"  x         cd into the folder and quit",
	"  c         open in VS Code",
	"  b         back to default branch + pull (selection or cursor row)",
	"  p         pull --ff-only (selection or cursor row)",
	"  P         pull all: every clean repo that is behind and not ahead",
	"  R         reset --hard to upstream (asks first)",
	"  v         view diff (uncommitted changes vs HEAD)",
	"  f         fetch all repos",
	"  r         re-read local state of all repos and re-sort",
	"  s         toggle sort: last updated / name",
	"  space     select / unselect row",
	"  →/←       unfold / fold worktrees (← on a child jumps to parent)",
	"  d         remove worktree (branch is kept) / delete repo folder (asks first)",
	"  /         filter by name (enter keeps, esc clears)",
	"  ?         show this help",
	"  q / esc   close overlay; quit when none is open",
}

func sty(fg string, cur bool) lipgloss.Style {
	if cur {
		return styBG(fg, cursorBG)
	}
	return styBG(fg, "")
}

func styBG(fg, bg string) lipgloss.Style {
	s := lipgloss.NewStyle()
	if fg != "" {
		s = s.Foreground(lipgloss.Color(fg))
	}
	if bg != "" {
		s = s.Background(lipgloss.Color(bg))
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
	switch c.kind {
	case kRemove:
		l = append(l, fmt.Sprintf("Remove worktree %s? Branch %s is kept.", c.repo.Path, c.repo.Status.Branch))
		if len(c.files) > 0 {
			l = append(l, "Uncommitted changes, these changes will be lost:")
		}
	case kDelete:
		st := c.repo.Status
		l = append(l, fmt.Sprintf("DELETE repo folder %s? All local branches, stashes and unpushed commits (↑%d) are lost.", c.repo.Path, st.Ahead))
		if n := len(c.repo.Worktrees); n > 0 {
			l = append(l, fmt.Sprintf("It has %d worktree(s), deleting them too:", n))
			for _, w := range c.repo.Worktrees {
				l = append(l, "  "+w.Path)
			}
		}
		if len(c.files) > 0 {
			l = append(l, "Uncommitted changes, these changes will be lost:")
		}
	case kReset:
		st := c.repo.Status
		l = append(l, fmt.Sprintf("Reset %s to %s? Local commits (↑%d) and uncommitted changes will be lost:", c.repo.Name, st.Upstream, st.Ahead))
	default:
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

func stateColor(s repo.Status) string {
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
	bg := ""
	switch {
	case cur:
		bg = cursorBG
	case m.sel[r.Path]:
		bg = selectedBG
	}
	sty := func(fg string, _ bool) lipgloss.Style { return styBG(fg, bg) }
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
		st, fg = repo.FirstLine(e), red
	}
	if m.busy[r.Path] {
		st, fg = "…", yellow
	}
	gap := sty("", cur).Render("  ")
	line := sty(purple, cur).Render(g0) + sty(green, cur).Render(g1) + sty("", cur).Render(" ") + nameCell + gap +
		sty("", cur).Render(pad(trunc(branchText(r.Status), c.branch), c.branch)) + gap +
		sty(fg, cur).Render(pad(trunc(st, c.state), c.state)) + gap +
		sty(grey, cur).Render(pad(relTime(now, r.Status.Updated), c.upd))
	if n := m.width() - lipgloss.Width(line); n > 0 {
		line += sty("", cur).Render(strings.Repeat(" ", n))
	}
	return line
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
	r := fmt.Sprintf("%d repos · sort: %s", len(m.repos), m.sort)
	switch {
	case m.loading:
		r += "   loading…"
	case m.fetching():
		r += fmt.Sprintf("   fetching… %d/%d", m.fetchDone, m.fetchTotal)
	}
	gap := max(1, m.width()-lipgloss.Width(l)-lipgloss.Width(r)-1)
	return lipgloss.NewStyle().Bold(true).Render(l) + strings.Repeat(" ", gap) + sty(grey, false).Render(r)
}

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m model) render() string {
	w := m.width()
	c := m.cols()
	var b []string
	b = append(b, m.header())
	b = append(b, sty(grey, false).Bold(true).Render(pad("   "+pad("REPO", c.name)+"  "+pad("BRANCH", c.branch)+"  "+pad("STATE", c.state)+"  UPDATED", w)))
	var body []string
	now := time.Now()
	rs := m.rows()
	for i := m.off; i < len(rs) && i < m.off+m.listH(); i++ {
		body = append(body, m.renderRow(rs[i], i == m.idx, c, now))
	}
	for len(body) < m.listH() && m.h > 0 {
		body = append(body, "")
	}
	if m.help {
		body = overlay(body, box(append(helpLines, "", " esc/q/? close"), w), w)
	}
	if m.menu != nil {
		body = overlay(body, m.menuBox(w), w)
	}
	if m.diff != nil {
		body = overlay(body, m.diffBox(w, len(body)), w)
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

// menuBox renders the menu as a bordered box, at most maxW wide.
func (m model) menuBox(maxW int) []string {
	return box(m.menuLines(), maxW)
}

// box renders lines as a bordered box, at most maxW wide.
func box(lines []string, maxW int) []string {
	b := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(purple)).
		Padding(0, 1).
		MaxWidth(maxW).
		Render(strings.Join(lines, "\n"))
	return strings.Split(b, "\n")
}

// overlay draws box centered over base, keeping base visible around it.
func overlay(base, box []string, w int) []string {
	bw := 0
	for _, l := range box {
		bw = max(bw, lipgloss.Width(l))
	}
	x := max(0, (w-bw)/2)
	y := max(0, (len(base)-len(box))/2)
	out := append([]string(nil), base...)
	for len(out) < y+len(box) {
		out = append(out, "")
	}
	for i, l := range box {
		bg := out[y+i]
		left := ansi.Truncate(bg, x, "")
		left += strings.Repeat(" ", max(0, x-lipgloss.Width(left)))
		out[y+i] = left + l + ansi.TruncateLeft(bg, x+lipgloss.Width(l), "")
	}
	return out
}
