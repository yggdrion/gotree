package repo

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

func firstLine(err error) string {
	l, _, _ := strings.Cut(err.Error(), "\n")
	return strings.TrimSpace(l)
}

func pullFF(path string) error {
	if _, err := gitNet(path, "pull", "--ff-only"); err != nil {
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "not possible to fast-forward") || strings.Contains(low, "diverg") {
			return errors.New("pull: not fast-forward")
		}
		return fmt.Errorf("pull: %s", firstLine(err))
	}
	return nil
}

// BackToDefault checks out the default branch and fast-forwards it; discard wipes local changes first.
func BackToDefault(r *Repo, discard bool) error {
	if r.IsWorktree {
		return errors.New("not allowed on a worktree")
	}
	if r.Status.Default == "" {
		return errors.New("unknown default branch")
	}
	if discard {
		if _, err := git(r.Path, "reset", "--hard"); err != nil {
			return fmt.Errorf("discard: %s", firstLine(err))
		}
		if _, err := git(r.Path, "clean", "-fd"); err != nil {
			return fmt.Errorf("discard: %s", firstLine(err))
		}
	}
	if _, err := git(r.Path, "checkout", r.Status.Default); err != nil {
		return fmt.Errorf("checkout: %s", firstLine(err))
	}
	return pullFF(r.Path)
}

// ResetToUpstream hard-resets the current branch to its upstream; untracked files are kept.
func ResetToUpstream(r *Repo) error {
	if r.Status.Upstream == "" {
		return errors.New("no upstream")
	}
	if r.Status.Gone {
		return errors.New("upstream gone")
	}
	if _, err := git(r.Path, "reset", "--hard", "@{upstream}"); err != nil {
		return fmt.Errorf("reset: %s", firstLine(err))
	}
	return nil
}

// CheckConfirmed refuses a destructive action if files appeared that the dialog never listed.
func CheckConfirmed(path string, confirmed []string) error {
	ok := map[string]bool{}
	for _, f := range confirmed {
		ok[f] = true
	}
	for _, f := range ReadStatus(path).Files {
		if !ok[f] {
			return errors.New("changed since confirm, press r")
		}
	}
	return nil
}

func Pull(r *Repo) error {
	if r.Status.Upstream == "" {
		return errors.New("no upstream")
	}
	if r.Status.Gone {
		return errors.New("upstream gone")
	}
	return pullFF(r.Path)
}

func Fetch(path string) error {
	if _, err := gitNet(path, "fetch", "--all", "--prune"); err != nil {
		return fmt.Errorf("fetch: %s", err)
	}
	if _, err := git(path, "symbolic-ref", "refs/remotes/origin/HEAD"); err != nil {
		if out, err := git(path, "remote"); err == nil {
			for _, l := range strings.Split(out, "\n") {
				if strings.TrimSpace(l) == "origin" {
					_, _ = gitNet(path, "remote", "set-head", "origin", "--auto") // error ignored
					break
				}
			}
		}
	}
	return nil
}

func RemoveWorktree(wt *Repo, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	if _, err := git(wt.MainPath, append(args, wt.Path)...); err != nil {
		return fmt.Errorf("worktree remove: %s", firstLine(err))
	}
	return nil
}

func OpenEditor(path string) error {
	bin, err := exec.LookPath("code")
	if err != nil {
		return errors.New("VS Code 'code' command not found on PATH")
	}
	cmd := exec.Command(bin, path)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open: %s", firstLine(err))
	}
	go func() { _ = cmd.Wait() }() // reap
	return nil
}

func FirstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(l)
}

// Diff returns uncommitted changes vs HEAD (staged and unstaged), followed by untracked files.
func Diff(path string) (string, error) {
	out, err := git(path, "diff", "HEAD", "--no-color", "--no-ext-diff")
	if err != nil { // no HEAD yet (unborn branch): diff against the index
		if out, err = git(path, "diff", "--no-color", "--no-ext-diff"); err != nil {
			return "", fmt.Errorf("diff: %s", firstLine(err))
		}
	}
	if u, err := git(path, "ls-files", "--others", "--exclude-standard"); err == nil && u != "" {
		if out != "" {
			out += "\n\n"
		}
		out += "untracked:\n"
		for _, f := range strings.Split(u, "\n") {
			out += "  ? " + f + "\n"
		}
		out = strings.TrimRight(out, "\n")
	}
	return out, nil
}
