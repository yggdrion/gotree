package main

import (
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"
)

func TestParseStatusV2(t *testing.T) {
	out := "# branch.oid abc123\n# branch.head feat/x\n# branch.upstream origin/feat/x\n# branch.ab +2 -4\n" +
		"1 .M N... 100644 100644 100644 aaa bbb config.json\n? notes.txt\n"
	b := parseStatusV2(out)
	if b.Head != "feat/x" || b.OID != "abc123" || b.Upstream != "origin/feat/x" || !b.HasAB || b.Ahead != 2 || b.Behind != 4 || b.Detached {
		t.Fatalf("got %+v", b)
	}
	if !reflect.DeepEqual(b.Files, []string{"config.json", "notes.txt"}) {
		t.Fatalf("files %v", b.Files)
	}
}

func TestParseStatusV2Gone(t *testing.T) {
	b := parseStatusV2("# branch.oid abc\n# branch.head x\n# branch.upstream origin/x\n")
	if b.HasAB || b.Upstream != "origin/x" {
		t.Fatalf("got %+v", b)
	}
}

func TestParseStatusV2Detached(t *testing.T) {
	b := parseStatusV2("# branch.oid abc\n# branch.head (detached)\n")
	if !b.Detached || b.OID != "abc" {
		t.Fatalf("got %+v", b)
	}
}

func TestParseDefaultBranch(t *testing.T) {
	for in, want := range map[string]string{"origin/master": "master", "origin/beta7": "beta7", "": ""} {
		if got := parseDefaultBranch(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestParseWorktreeList(t *testing.T) {
	out := "worktree C:/w/Squad\nHEAD aaa\nbranch refs/heads/master\n\n" +
		"worktree C:/w/Squad/.claude/worktrees/fix\nHEAD bbb\nbranch refs/heads/claude/fix\n\n" +
		"worktree C:/w/gone\nHEAD ccc\ndetached\nprunable gitdir file points to non-existent location\n"
	l := parseWorktreeList(out)
	if len(l) != 3 {
		t.Fatalf("len %d", len(l))
	}
	if l[0].Branch != "master" || l[0].Prunable {
		t.Errorf("%+v", l[0])
	}
	if l[1].Path != normPath("C:/w/Squad/.claude/worktrees/fix") || l[1].Branch != "claude/fix" {
		t.Errorf("%+v", l[1])
	}
	if !l[2].Prunable {
		t.Errorf("%+v", l[2])
	}
}

func TestNormPath(t *testing.T) {
	if normPath("a/b/../c") != filepath.Join("a", "c") {
		t.Error("clean")
	}
	if runtime.GOOS == "windows" && normPath("C:/x/Y") != normPath(`c:\x\Y`) {
		t.Error("drive case")
	}
}

func TestGitEnvNoPrompt(t *testing.T) {
	e := gitEnv()
	if !slices.Contains(e, "GIT_TERMINAL_PROMPT=0") || !slices.Contains(e, "GCM_INTERACTIVE=never") {
		t.Error("missing env")
	}
}
