package main

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
	if _, err := git(path, "pull", "--ff-only"); err != nil {
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
	if _, err := git(path, "fetch", "--all", "--prune"); err != nil {
		return fmt.Errorf("fetch: %s", firstLine(err))
	}
	if _, err := git(path, "symbolic-ref", "refs/remotes/origin/HEAD"); err != nil {
		if out, err := git(path, "remote"); err == nil {
			for _, l := range strings.Split(out, "\n") {
				if strings.TrimSpace(l) == "origin" {
					git(path, "remote", "set-head", "origin", "--auto") // error ignored
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
	go cmd.Wait() // reap
	return nil
}
