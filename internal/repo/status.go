package repo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Status struct {
	Branch        string // "" when detached
	Detached      bool
	SHA           string // short (7) sha, used when Detached
	Upstream      string // "" = no upstream
	Gone          bool   // upstream configured but missing
	Ahead, Behind int
	Files         []string  // uncommitted, incl. untracked
	Default       string    // "" = unknown -> UI shows "?"
	Merged        bool      // not on Default and HEAD is ancestor of origin/<Default>
	Updated       time.Time // max(last commit time, newest mtime among uncommitted files)
	Err           string    // last load/action error, short
}

// ReadStatus never panics; git errors land in Err and partial state is kept.
func ReadStatus(path string) Status {
	var s Status
	out, err := git(path, "--no-optional-locks", "-c", "core.quotePath=false", "status", "--porcelain=v2", "--branch")
	if err != nil {
		s.Err, _, _ = strings.Cut(err.Error(), "\n")
		return s
	}
	b := parseStatusV2(out)
	s.Branch, s.Detached, s.Upstream = b.Head, b.Detached, b.Upstream
	s.Gone = b.Upstream != "" && !b.HasAB
	s.Ahead, s.Behind, s.Files = b.Ahead, b.Behind, b.Files
	if len(b.OID) >= 7 && b.OID != "(initial)" {
		s.SHA = b.OID[:7]
	}

	if ref, err := git(path, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		s.Default = parseDefaultBranch(ref)
	}
	if s.Default != "" && !s.Detached && s.Branch != s.Default {
		// non-zero exit (not ancestor, or no such ref) just means not merged
		if _, err := git(path, "merge-base", "--is-ancestor", "HEAD", "origin/"+s.Default); err == nil {
			s.Merged = true
		}
	}

	if ct, err := git(path, "log", "-1", "--format=%ct"); err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(ct), 10, 64); err == nil {
			s.Updated = time.Unix(n, 0)
		}
	}
	if t := newestMod(path, s.Files); t.After(s.Updated) {
		s.Updated = t
	}
	return s
}

// maxStat bounds how many uncommitted files are stat'ed per repo.
const maxStat = 200

// newestMod returns the newest mtime among files (paths relative to dir).
// Missing files (deleted, renamed) are skipped. Unlike .git/index it is
// not touched by git's own housekeeping.
func newestMod(dir string, files []string) time.Time {
	var newest time.Time
	for i, f := range files {
		if i >= maxStat {
			break
		}
		if fi, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(f))); err == nil && fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
	}
	return newest
}
