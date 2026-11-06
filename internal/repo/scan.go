package repo

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Repo struct {
	Path, Name string // Name = folder base name
	CommonDir  string // normPath(rev-parse --path-format=absolute --git-common-dir)
	IsWorktree bool   // linked worktree (shown as child, or top-level if its main isn't listed)
	MainPath   string // for worktrees: main checkout path (first entry of worktree list)
	Status     Status
	Worktrees  []*Repo // linked worktrees of this repo, sorted Updated desc
	Expanded   bool    // UI fold state, default false
}

// SamePath compares paths after normPath and symlink resolution (macOS /var vs /private/var);
// case-insensitive on Windows.
func SamePath(a, b string) bool {
	res := func(p string) string {
		p = normPath(p)
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = normPath(r)
		}
		return p
	}
	a, b = res(a), res(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// QuickOrder lists direct child dirs of root containing .git, newest mtime first. No git calls.
func QuickOrder(root string) []string {
	ents, _ := os.ReadDir(root)
	type item struct {
		path string
		mod  time.Time
	}
	var items []item
	for _, e := range ents {
		p := filepath.Join(root, e.Name())
		if !e.IsDir() {
			continue
		}
		g, err := os.Stat(filepath.Join(p, ".git"))
		if err != nil {
			continue
		}
		mod := g.ModTime()
		for _, c := range []string{p, filepath.Join(p, ".git", "index")} {
			if fi, err := os.Stat(c); err == nil && fi.ModTime().After(mod) {
				mod = fi.ModTime()
			}
		}
		items = append(items, item{p, mod})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.path
	}
	return out
}

// LoadRepo returns nil unless path is the toplevel of a non-bare repo.
func LoadRepo(path string) *Repo {
	top, err := git(path, "rev-parse", "--show-toplevel")
	if err != nil || !SamePath(top, path) {
		return nil
	}
	common, err := git(path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil
	}
	r := &Repo{Path: normPath(path), Name: filepath.Base(path), CommonDir: normPath(common)}
	if fi, err := os.Stat(filepath.Join(path, ".git")); err == nil && !fi.IsDir() {
		r.IsWorktree = true
	}
	r.Status = ReadStatus(path)
	list, err := git(path, "worktree", "list", "--porcelain")
	if err != nil {
		return r
	}
	wts := parseWorktreeList(list)
	if len(wts) == 0 {
		return r
	}
	if r.IsWorktree {
		r.MainPath = wts[0].Path
		return r
	}
	for _, w := range wts[1:] {
		if w.Bare || w.Prunable {
			continue
		}
		if _, err := os.Stat(w.Path); err != nil { // older git doesn't flag missing dirs prunable
			continue
		}
		r.Worktrees = append(r.Worktrees, &Repo{
			Path: w.Path, Name: filepath.Base(w.Path), CommonDir: r.CommonDir,
			IsWorktree: true, MainPath: r.Path, Status: ReadStatus(w.Path),
		})
	}
	SortRepos(r.Worktrees)
	return r
}

// Dedupe drops top-level linked worktrees whose main repo is also listed. Keeps order.
func Dedupe(rs []*Repo) []*Repo {
	var out []*Repo
	for _, r := range rs {
		if r.IsWorktree && hasMain(rs, r.CommonDir) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func hasMain(rs []*Repo, commonDir string) bool {
	for _, m := range rs {
		if !m.IsWorktree && SamePath(m.CommonDir, commonDir) {
			return true
		}
	}
	return false
}

// SortRepos sorts by Status.Updated, newest first (stable).
func SortRepos(rs []*Repo) {
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].Status.Updated.After(rs[j].Status.Updated) })
}
