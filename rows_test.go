package main

import (
	"reflect"
	"testing"
	"time"
)

func TestVisibleRows(t *testing.T) {
	fix, trixie := &Repo{Name: "fix", IsWorktree: true}, &Repo{Name: "trixie", IsWorktree: true}
	squad := &Repo{Name: "Squad", Worktrees: []*Repo{fix, trixie}}
	infra := &Repo{Name: "infra"}
	rs := []*Repo{squad, infra}
	names := func(rows []row) []string {
		var o []string
		for _, r := range rows {
			n := r.Repo.Name
			if r.Child {
				n += "(child)"
			}
			o = append(o, n)
		}
		return o
	}
	tests := []struct {
		name     string
		expanded bool
		filter   string
		want     []string
	}{
		{"no filter folded", false, "", []string{"Squad", "infra"}},
		{"no filter expanded", true, "", []string{"Squad", "fix(child)", "trixie(child)", "infra"}},
		{"child match folded", false, "trix", []string{"Squad", "trixie(child)"}},
		{"child match expanded", true, "trix", []string{"Squad", "trixie(child)"}},
		{"parent match folded", false, "squad", []string{"Squad"}},
		{"parent match expanded", true, "squad", []string{"Squad", "fix(child)", "trixie(child)"}},
		{"case-insensitive", false, "INFRA", []string{"infra"}},
		{"no match", false, "zzz", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			squad.Expanded = tc.expanded
			if got := names(visibleRows(rs, tc.filter)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestVisibleRowsWorktreeMatchWhenParentMatches(t *testing.T) {
	sf := &Repo{Name: "squad-fix", IsWorktree: true}
	other := &Repo{Name: "other", IsWorktree: true}
	squad := &Repo{Name: "Squad", Worktrees: []*Repo{sf, other}}
	got := visibleRows([]*Repo{squad}, "squad")
	want := []row{{squad, false}, {sf, true}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestStateText(t *testing.T) {
	tests := []struct {
		name string
		s    Status
		want string
	}{
		{"clean", Status{Upstream: "origin/x", Default: "main"}, "✓"},
		{"ahead+files", Status{Upstream: "u", Default: "main", Ahead: 2, Files: []string{"a", "b", "c"}}, "↑2 ●3"},
		{"behind", Status{Upstream: "u", Default: "main", Behind: 4}, "↓4"},
		{"merged", Status{Upstream: "u", Default: "main", Merged: true}, "merged → main"},
		{"gone", Status{Upstream: "u", Default: "main", Gone: true}, "gone"},
		{"err", Status{Err: "pull: not fast-forward", Ahead: 1}, "! pull: not fast-forward"},
		{"err unknown default", Status{Err: "boom"}, "! boom"},
		{"no upstream", Status{Default: "main"}, "no upstream"},
		{"detached no upstream text", Status{Default: "main", Detached: true}, "✓"},
		{"unknown default alone", Status{Upstream: "u"}, "?"},
		{"unknown default with ahead", Status{Upstream: "u", Ahead: 2}, "? ↑2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stateText(tc.s); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestBranchText(t *testing.T) {
	if got := branchText(Status{Branch: "main"}); got != "main" {
		t.Errorf("got %q", got)
	}
	if got := branchText(Status{Detached: true, SHA: "abc1234"}); got != "(detached abc1234)" {
		t.Errorf("got %q", got)
	}
}

func TestRelTime(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"30s", now.Add(-30 * time.Second), "now"},
		{"future", now.Add(time.Hour), "now"},
		{"5m", now.Add(-5 * time.Minute), "5m"},
		{"3h", now.Add(-3 * time.Hour), "3h"},
		{"50h", now.Add(-50 * time.Hour), "2d"},
		{"zero", time.Time{}, "-"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := relTime(now, tc.t); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
