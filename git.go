package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func gitEnv() []string {
	return append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
}

// git runs `git -C dir args...`; stdout is right-trimmed, errors carry stderr text.
func git(dir string, args ...string) (string, error) {
	return run(exec.Command("git", append([]string{"-C", dir}, args...)...))
}

// netTimeout bounds network git calls (var so tests can shorten it).
var netTimeout = 2 * time.Minute

// gitNet is git for network ops: bounded by netTimeout and ssh never prompts on the tty.
func gitNet(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.WaitDelay = time.Second // don't wait on ssh children holding the pipes
	_, set1 := os.LookupEnv("GIT_SSH_COMMAND")
	_, set2 := os.LookupEnv("GIT_SSH")
	if !set1 && !set2 {
		if c, _ := git(dir, "config", "--get", "core.sshCommand"); c == "" {
			cmd.Env = append(gitEnv(), "GIT_SSH_COMMAND=ssh -o BatchMode=yes")
		}
	}
	out, err := run(cmd)
	if ctx.Err() == context.DeadlineExceeded {
		return "", &gitError{"timed out after " + netTimeout.String()}
	}
	return out, err
}

func run(cmd *exec.Cmd) (string, error) {
	if cmd.Env == nil {
		cmd.Env = gitEnv()
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", &gitError{msg}
		}
		return "", err
	}
	return strings.TrimRightFunc(stdout.String(), unicode.IsSpace), nil
}

type gitError struct{ msg string }

func (e *gitError) Error() string { return e.msg }

func normPath(p string) string {
	p = filepath.Clean(filepath.FromSlash(p))
	if runtime.GOOS == "windows" && len(p) >= 2 && p[1] == ':' {
		p = strings.ToLower(p[:1]) + p[1:]
	}
	return p
}

type branchInfo struct {
	Head, OID, Upstream string
	Detached, HasAB     bool
	Ahead, Behind       int
	Files               []string
}

func parseStatusV2(out string) branchInfo {
	var b branchInfo
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "#") {
			b.Files = append(b.Files, statusPath(line))
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		switch f[1] {
		case "branch.oid":
			b.OID = f[2]
		case "branch.head":
			if f[2] == "(detached)" {
				b.Detached = true
			} else {
				b.Head = f[2]
			}
		case "branch.upstream":
			b.Upstream = f[2]
		case "branch.ab":
			if len(f) >= 4 {
				b.HasAB = true
				b.Ahead, _ = strconv.Atoi(strings.TrimPrefix(f[2], "+"))
				b.Behind, _ = strconv.Atoi(strings.TrimPrefix(f[3], "-"))
			}
		}
	}
	return b
}

// statusPath extracts the path; split by field count so paths with spaces survive.
func statusPath(line string) string {
	if strings.HasPrefix(line, "? ") || strings.HasPrefix(line, "! ") {
		return line[2:]
	}
	n := map[byte]int{'1': 9, '2': 10, 'u': 11}[line[0]]
	if n == 0 {
		f := strings.Fields(line)
		return f[len(f)-1]
	}
	f := strings.SplitN(line, " ", n)
	p := f[len(f)-1]
	if line[0] == '2' { // rename: "new\torig"
		p, _, _ = strings.Cut(p, "\t")
	}
	return p
}

func parseDefaultBranch(symref string) string {
	_, b, _ := strings.Cut(strings.TrimSpace(symref), "/")
	return b
}

type wtEntry struct {
	Path, Branch   string
	Bare, Prunable bool
}

func parseWorktreeList(out string) []wtEntry {
	var res []wtEntry
	for _, block := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n\n") {
		var e wtEntry
		for _, line := range strings.Split(block, "\n") {
			k, v, _ := strings.Cut(line, " ")
			switch k {
			case "worktree":
				e.Path = normPath(v)
			case "branch":
				e.Branch = strings.TrimPrefix(v, "refs/heads/")
			case "bare":
				e.Bare = true
			case "prunable":
				e.Prunable = true
			}
		}
		if e.Path != "" {
			res = append(res, e)
		}
	}
	return res
}
