package repo

import (
	"os"
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
	Updated       time.Time // max(last commit time, mtime of index)
	Err           string    // last load/action error, short
}

// ReadStatus never panics; git errors land in Err and partial state is kept.
func ReadStatus(path string) Status {
	var s Status
	out, err := git(path, "-c", "core.quotePath=false", "status", "--porcelain=v2", "--branch")
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
	if idx, err := git(path, "rev-parse", "--path-format=absolute", "--git-path", "index"); err == nil {
		if fi, err := os.Stat(idx); err == nil && fi.ModTime().After(s.Updated) {
			s.Updated = fi.ModTime()
		}
	}
	return s
}
