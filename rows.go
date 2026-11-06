package main

import (
	"strconv"
	"strings"
	"time"
)

type row struct {
	Repo  *Repo
	Child bool
}

func matches(r *Repo, f string) bool {
	return f == "" || strings.Contains(strings.ToLower(r.Name), f)
}

// visibleRows flattens repos and their worktrees, applying fold state and a
// case-insensitive name filter. Matching worktrees show even when folded,
// unless the repo itself matches (then folded means no children).
func visibleRows(rs []*Repo, filter string) []row {
	f := strings.ToLower(filter)
	var out []row
	for _, r := range rs {
		self := matches(r, f)
		var kids []row
		for _, w := range r.Worktrees {
			switch {
			case r.Expanded && self, matches(w, f) && (r.Expanded || !self):
				kids = append(kids, row{w, true})
			}
		}
		if self || len(kids) > 0 {
			out = append(out, row{Repo: r})
			out = append(out, kids...)
		}
	}
	return out
}

func stateText(s Status) string {
	if s.Err != "" {
		return "! " + s.Err
	}
	var p []string
	if s.Ahead > 0 {
		p = append(p, "↑"+strconv.Itoa(s.Ahead))
	}
	if s.Behind > 0 {
		p = append(p, "↓"+strconv.Itoa(s.Behind))
	}
	if len(s.Files) > 0 {
		p = append(p, "●"+strconv.Itoa(len(s.Files)))
	}
	if s.Upstream == "" && !s.Detached {
		p = append(p, "no upstream")
	}
	if s.Gone {
		p = append(p, "gone")
	}
	if s.Merged {
		p = append(p, "merged → "+s.Default)
	}
	if len(p) == 0 && s.Default != "" {
		p = append(p, "✓")
	}
	if s.Default == "" {
		p = append([]string{"?"}, p...)
	}
	return strings.Join(p, " ")
}

func branchText(s Status) string {
	if s.Detached {
		return "(detached " + s.SHA + ")"
	}
	return s.Branch
}

func relTime(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d/time.Hour)) + "h"
	}
	return strconv.Itoa(int(d/(24*time.Hour))) + "d"
}
