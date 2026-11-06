package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestReadStatusClean(t *testing.T) {
	_, clone := newFixture(t)
	s := ReadStatus(clone)
	if s.Branch != "beta7" || s.Default != "beta7" || s.Ahead != 0 || s.Behind != 0 || len(s.Files) != 0 || s.Err != "" || s.Updated.IsZero() || s.Merged {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestReadStatusAheadDirty(t *testing.T) {
	_, clone := newFixture(t)
	gitT(t, clone, "commit", "--allow-empty", "-m", "local")
	os.WriteFile(filepath.Join(clone, "new.txt"), []byte("x"), 0o644)
	s := ReadStatus(clone)
	if s.Ahead != 1 || s.Merged || !reflect.DeepEqual(s.Files, []string{"new.txt"}) {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestReadStatusBehind(t *testing.T) {
	remote, clone := newFixture(t)
	other := cloneOf(t, remote)
	gitT(t, other, "commit", "--allow-empty", "-m", "remote")
	gitT(t, other, "push")
	gitT(t, clone, "fetch")
	if s := ReadStatus(clone); s.Behind != 1 {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestReadStatusMerged(t *testing.T) {
	_, clone := newFixture(t)
	gitT(t, clone, "checkout", "-b", "feat")
	if s := ReadStatus(clone); !s.Merged || s.Branch != "feat" {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestReadStatusGone(t *testing.T) {
	remote, clone := newFixture(t)
	gitT(t, clone, "checkout", "-b", "feat")
	gitT(t, clone, "push", "-u", "origin", "feat")
	gitT(t, remote, "branch", "-D", "feat")
	gitT(t, clone, "fetch", "--prune")
	if s := ReadStatus(clone); !s.Gone || s.Upstream != "origin/feat" {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestReadStatusDetached(t *testing.T) {
	_, clone := newFixture(t)
	gitT(t, clone, "checkout", "--detach")
	if s := ReadStatus(clone); !s.Detached || len(s.SHA) != 7 || s.Branch != "" || s.Merged {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestReadStatusEmptyRepo(t *testing.T) {
	isolateGit(t)
	dir := t.TempDir()
	gitT(t, dir, "init")
	if s := ReadStatus(dir); s.Default != "" || s.Err != "" {
		t.Fatalf("unexpected: %+v", s)
	}
}

func TestReadStatusUnquotesPaths(t *testing.T) {
	_, clone := newFixture(t)
	os.WriteFile(filepath.Join(clone, "über.txt"), []byte("x"), 0o644)
	if s := ReadStatus(clone); !reflect.DeepEqual(s.Files, []string{"über.txt"}) {
		t.Fatalf("files = %q", s.Files)
	}
}
