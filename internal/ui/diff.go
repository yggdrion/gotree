package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yggdrion/gotree/internal/repo"
)

type diffState struct {
	r     *repo.Repo
	lines []string
	off   int
}

func (m *model) openDiff(r *repo.Repo) {
	out, err := repo.Diff(r.Path)
	if err != nil {
		m.setStatus(false, "%s", err)
		return
	}
	if out == "" {
		m.setStatus(true, "%s: no changes", r.Name)
		return
	}
	m.diff = &diffState{r: r, lines: strings.Split(strings.ReplaceAll(out, "\t", "    "), "\n")}
}

// diffPage is how many content lines fit in the overlay.
func (m model) diffPage() int {
	return max(1, m.listH()-3) // border top/bottom + hint line
}

func (m *model) diffKey(s string) {
	d := m.diff
	page := m.diffPage()
	switch s {
	case "esc", "q", "v":
		m.diff = nil
		return
	case "up", "k":
		d.off--
	case "down", "j":
		d.off++
	case "pgup", "b", "ctrl+u":
		d.off -= page
	case "pgdown", "space", "f", "ctrl+d":
		d.off += page
	case "home", "g":
		d.off = 0
	case "end", "G":
		d.off = len(d.lines)
	}
	d.off = max(0, min(d.off, len(d.lines)-page))
}

func diffColor(l string) string {
	switch {
	case strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "---"),
		strings.HasPrefix(l, "diff "), strings.HasPrefix(l, "index "):
		return grey
	case strings.HasPrefix(l, "+"):
		return green
	case strings.HasPrefix(l, "-"):
		return red
	case strings.HasPrefix(l, "@@"):
		return purple
	case strings.HasPrefix(l, "untracked:"), strings.HasPrefix(l, "  ? "):
		return yellow
	}
	return ""
}

// diffBox renders the diff as a bordered, scrollable box almost as large as the list area.
func (m model) diffBox(maxW, bodyH int) []string {
	d := m.diff
	inner := max(10, maxW-6) // border + padding + 1 col margin each side
	page := m.diffPage()
	if bodyH > 0 {
		page = min(page, max(1, bodyH-3))
	}
	page = min(page, len(d.lines))
	var l []string
	for i := d.off; i < d.off+page && i < len(d.lines); i++ {
		s := ansi.Truncate(d.lines[i], inner, "…")
		l = append(l, pad(sty(diffColor(d.lines[i]), false).Render(s), inner))
	}
	hint := fmt.Sprintf("%s  %d-%d/%d · j/k scroll · space/b page · g/G top/end · esc close",
		d.r.Name, d.off+1, min(d.off+page, len(d.lines)), len(d.lines))
	l = append(l, sty(grey, false).Render(pad(ansi.Truncate(hint, inner, "…"), inner)))
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(purple)).
		Padding(0, 1).
		Render(strings.Join(l, "\n"))
	return strings.Split(box, "\n")
}
