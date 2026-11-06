# gotree: design

Date: 2026-11-06

## Intent

`~/workspace` is a flat folder of ~60 independent git checkouts. Some have Claude Code worktrees under
`<repo>/.claude/worktrees/*`. The routine chore: after a PR merges, go back to the repo's default branch
and pull. Today that means `cd` into each repo, `git checkout <default>`, `git pull`. That gets slow across
many repos. Picking which repo or worktree to jump into is also slow.

gotree is a k9s-style terminal UI (Go + Bubble Tea). It shows every repo in the folder it was started from,
with its git state at a glance. Common git chores are one key each. Enter drops you back into the shell,
in the chosen folder.

## Goals

1. List all git repos that are **direct children** of the start folder, sorted by last updated (newest first).
2. Per row, show: name, current branch, ahead/behind of upstream, uncommitted file count, merged hint, last updated.
3. One key, **back to default + pull**: check out the repo's default branch, then pull. Works on the cursor row
   or on all selected rows.
4. Separate keys for plain pull and fetch (all repos).
5. Show worktrees as child rows under their repo. Folded by default. Can be unfolded, jumped into and deleted.
6. Enter on a repo or worktree quits and leaves the user's shell `cd`'d into that folder. `c` opens it in VS Code instead and stays in the app.
7. `/` filters rows by name.

## Non-goals

- No commit, push, rebase, merge, stash or branch-picker UI. The user runs those by hand after `cd`.
- No recursive scan. Only direct children of the start folder.
- No config file in v1.
- No automatic cleanup of anything. The app only removes something when the user presses a key and confirms.

## Anti-goals (would make it a failure even if it "works")

- Losing work silently. Every destructive action (discard changes, remove worktree) needs an explicit confirm
  that names what will be lost.
- Surprise merge commits. Pull is always `git pull --ff-only`.
- Deleting branches. The user said: do not clean up, just show merged state. Removing a worktree leaves its
  branch alone.
- Slow startup. The list must render from local state at once. Network work (fetch) runs in the background.

## Constraints

- Primary platform: Windows 11, PowerShell, Go 1.27. Must build and run on Linux and macOS too (see Platforms).
- Shell out to the `git` CLI (`os/exec`), not a Go git library. It matches the user's own git behavior,
  credentials and config.
- Dependencies: `github.com/charmbracelet/bubbletea`, `bubbles` (textinput for the filter, optional),
  `lipgloss`. Nothing else unless needed.
- Module path: `github.com/yggdrion/gotree`.

## Platforms, CI and release (user's decision: Linux, Windows, macOS; Homebrew for mac)

- Targets: `linux/{amd64,arm64}`, `windows/{amd64,arm64}`, `darwin/{amd64,arm64}`. Pure Go, `CGO_ENABLED=0`.
  No OS-specific code paths except: the `.exe` suffix (handled by the build), and path handling, which must use
  `path/filepath` everywhere. Git output paths are forward-slash on Windows, so normalize them with
  `filepath.FromSlash`/`filepath.Clean` before comparing or deduping.
- Repo: `github.com/yggdrion/gotree`, so the module path is `github.com/yggdrion/gotree`.
- Follow the conventions of the user's `gote` repo: emoji workflow names, actions pinned to a full commit SHA
  with a `# vX.Y.Z` comment, and `secrets.HOMEBREW_TAP_TOKEN` (already used for the tap).
- **CI workflow** (`.github/workflows/ci.yml`, on push to `main` and on PRs): matrix on `ubuntu-latest`,
  `windows-latest`, `macos-latest`. It runs `go vet ./...`, `go test ./...` (including the git integration test,
  so git must be on PATH, which all three runners have), and `go build ./...`. Plus one golangci-lint job on
  ubuntu.
- **Release workflow** (`.github/workflows/release.yml`). Trigger: `release: types: [published]`. The user creates
  the GitHub release (and tag `vX.Y.Z`) by hand in the UI, and this workflow then builds and attaches the
  binaries. It uses GoReleaser v2 (`goreleaser/goreleaser-action`) with `.goreleaser.yaml`:
  - `release.mode: append` so it adds assets to the existing, hand-made release and does not fail on it.
  - Archives: `tar.gz` for linux/darwin, `zip` for windows, plus `checksums.txt`.
  - Version injected with `-ldflags "-X main.version={{.Version}}"`. `gotree --version` prints it.
  - Homebrew: GoReleaser `homebrew_casks` pushes `Casks/gotree.rb` to `yggdrion/homebrew-tap`. The tap already
    has `Casks/gote.rb`. Repository token: `HOMEBREW_TAP_TOKEN`. The binary is unsigned, so the cask needs a
    postflight hook that strips the `com.apple.quarantine` xattr (GoReleaser documents this pattern). Otherwise
    Gatekeeper blocks it.
  - Install for the user: `brew install yggdrion/tap/gotree`.
- Windows and Linux users download from the release page. No Scoop, winget or deb packages in v1.

## Behavior details (decided with the user)

### Default branch

Per repo, read from `git symbolic-ref --short refs/remotes/origin/HEAD` (for example `origin/master` gives
`master`). Repos use `main`, `master`, `beta7`, and so on. Never hardcode `main`.
Fallback if `origin/HEAD` is unset: try `git remote set-head origin --auto` once, then re-read. If there is
still nothing, show `?` and refuse back-to-default on that row with a status message.

### Row state

- `↑N` = commits ahead of upstream (needs push). `↓N` = behind (needs pull). `●N` = uncommitted files
  (from `git status --porcelain`, untracked included). `✓` = clean and in sync.
- **merged** hint: the current branch is not the default branch, and either
  (a) it is an ancestor of `origin/<default>` (`git merge-base --is-ancestor HEAD origin/<default>`), or
  (b) its upstream is gone after a fetch with prune (GitHub squash-merge, then branch deleted).
  (b) shows as `gone`, (a) shows as `merged → <default>`. Both mean "probably safe to go back".
- No upstream: show `no upstream` instead of ↑/↓.
- **Last updated** = newest of: last commit time (`git log -1 --format=%ct`) and mtime of `.git/index`
  (catches checkouts and staging). For worktrees, use the worktree's own HEAD and index.
  Display as relative time (`2m`, `3h`, `5d`).
- While an action runs on a row, show a spinner or `…` in STATE. On error, show a short red error on the row.
  The full message goes in the status line.

### Back to default + pull (`b`)

1. If the working tree is dirty, show a confirm dialog that lists the changed files (from porcelain,
   truncated with "+N more"). `y` discards and continues: `git reset --hard` then `git clean -fd`
   (untracked removed; ignored files like `.env` and `node_modules` kept, so never `-x`).
   Any other key cancels that repo.
2. `git checkout <default>`.
3. `git pull --ff-only`.
4. Refresh the row.

With several rows selected, run them in parallel. Dirty repos are asked about one at a time, before
anything runs. Clean ones just run.

Worktrees: `b` on a worktree row is not allowed (the default branch is usually checked out in the main
repo, so git refuses). Show a status message.

### Pull (`p`), fetch (`f`)

- `p`: `git pull --ff-only` on the cursor row or the selection.
- `f`: `git fetch --all --prune` on **all** repos in parallel (bounded, for example 8 at a time). Show progress
  in the header (`fetching… 41/62`). This also runs once, automatically, at startup.
- `r`: re-read local state of all repos (no network).

### Work order: most recently touched first (user's decision)

At startup, first do a cheap pass with no git calls: for each child folder, take the newest mtime of the folder,
`.git` and `.git/index`, and sort by that, newest first. Everything after this follows that order:
- Local state loading. Rows appear progressively, newest first, so the repos the user just worked in show up
  and fill in before the rest.
- The startup fetch and `f`. Workers pick repos from the queue in that order, so recent repos are fresh first.
- Worktree state inside a repo, also newest first.
Once local state is loaded, the displayed sort switches to the precise "last updated" value from Row state.


### Worktrees

- Discovered with `git worktree list --porcelain` per repo (catches `.claude/worktrees/*` and any other
  location). The main checkout itself is not a child row.
- Repo row shows a `⎇N` badge when N > 0. Folded by default. `→` unfolds, `←` folds (on a child row, `←` jumps
  to the parent).
- Worktree row shows the folder name, branch, the same state columns, and the merged hint.
- `d` on a worktree row: confirm dialog ("Remove worktree <path>? Branch <b> is kept."). If dirty, the dialog
  also lists the changes and says they will be lost. `y` runs `git worktree remove [--force] <path>` from the
  main repo. **Never deletes the branch.**
- `d` on a repo row does nothing.
- Top-level folders that are themselves linked worktrees of another repo (`.git` is a file, not a dir):
  hide them from the top level if their main repo is also in the list (they appear as its children).
  Otherwise list them normally.

### Enter: cd into a folder

The TUI cannot change its parent shell's directory. So:
- On Enter, gotree writes the chosen absolute path to a temp file, then quits. The file is named by env var
  `GOTREE_CD_FILE`. If that var is unset, gotree prints the path to stdout after the TUI exits.
- A shell wrapper sets that env var, runs gotree, and `cd`s to the path if the file is non-empty.
- README ships both wrappers. PowerShell (`$PROFILE`):
  ```powershell
  function gt { $f = New-TemporaryFile; $env:GOTREE_CD_FILE = $f; gotree.exe @args; $p = Get-Content $f -Raw -Encoding UTF8; Remove-Item $f; $env:GOTREE_CD_FILE = $null; if ($p) { Set-Location $p.Trim() } }
  ```
  bash/zsh:
  ```bash
  gt() { local f; f=$(mktemp); GOTREE_CD_FILE=$f gotree "$@"; local p; p=$(cat "$f"); rm -f "$f"; [ -n "$p" ] && cd "$p"; }
  ```
- `q` / `Esc` / `Ctrl+C` quit without writing.

### Open in VS Code (`c`) (user's addition)

`c` on a repo or worktree row runs `code <path>` and does not quit gotree. The process is started detached
(`cmd.Start()`, not waited on), so the TUI never blocks. Resolve `code` via `exec.LookPath`, which finds `code.cmd`
through PATHEXT on Windows. If it is not found, the status line shows `VS Code 'code' command not found on PATH`.
On success the status line shows `opened <name> in VS Code`.

### Filter (`/`)

Typing narrows rows by case-insensitive substring on the repo name. A worktree row matches on its own
name too, and a match shows its parent. `Esc` clears the filter. `Enter` keeps the filter and returns focus
to the list.

### Selection

`x` toggles selection on a row (marked `✓` in a left gutter). `b` and `p` act on the selection if one exists,
else on the cursor row. Selection clears after the action.

### Keys (shown in a footer line; `?` toggles a full help view)

`↑/↓ j/k` move · `enter` cd · `c` open in VS Code · `b` back-to-default+pull · `p` pull · `f` fetch all · `r` refresh · `x` select ·
`→/←` fold · `d` delete worktree · `/` filter · `?` help · `q` quit

## Layout (approved mockup "A v2")

```
 gotree  ~/workspace                         62 repos   fetching… 41/62
   REPO                     BRANCH                     STATE              UPDATED
   gotree                   main                       ✓                  2m
 ✓ SquadJS                  feat/teamkill-log          ↑2 ●3              1h
▾  Squad  ⎇5                master                     ↓4                 3h
   └ steamcmd-update-fix    claude/steamcmd-...        ↑1                 3h
   └ trixie-docker-steam    claude/trixie-...          ✓ merged           1d
 ▸ ChopChop ⎇3              main                       ↓12                5d
   ServerConfig             fix/admins                 merged → main      2d
 ───────────────────────────────────────────────────────────────────────────
 enter cd · b back-to-default+pull · p pull · f fetch · x select · →/← fold · d delete worktree · / filter · ?
```

Single full-width table with a cursor highlight. Columns size to terminal width, and the branch is truncated
with `...`. Status/error line sits above the footer. Colors: green clean, yellow ahead/behind, red dirty and
errors, purple worktrees, grey hints.

## Calls made after builder review (Claude's calls, not discussed with the user)

1. **Repo identity / dedupe.** A child folder is a repo if `git rev-parse --show-toplevel` succeeds and the folder
   is that toplevel. Bare repos and non-git folders are skipped. Main repo of any checkout =
   `git rev-parse --path-format=absolute --git-common-dir`. Dedupe by common dir: a top-level folder that is a
   linked worktree whose main repo is also listed is hidden (it appears as a child). If its main repo is not
   listed, it shows as a normal top-level row. Submodules are not scanned.
2. **Detached HEAD.** BRANCH shows `(detached <short sha>)`. `b` is allowed. `git checkout <default>` relies on
   git's auto-tracking when only `origin/<default>` exists. If checkout fails (for example the default branch is
   checked out in another worktree), show the git error on the row and stop.
3. **Failed ff pull in `b`.** The repo stays on the default branch. The row shows a red `pull: not fast-forward`,
   with the full error in the status line. Nothing else is attempted.
4. **`p` with no upstream or a gone upstream.** Refused with a status message, and git is not called.
   `b` does not fetch separately (`git pull` fetches).
5. **Dirty confirm with a selection.** Each dirty repo gets its own dialog in turn: `y` discards that repo,
   `n` or any other key skips only that repo, `Esc` cancels the whole batch before anything runs. The dialog
   lists staged changes too (they are lost as well). Nested repos inside untracked folders are left alone
   (`clean -fd`, not `-ffd`).
6. **cd file encoding.** gotree writes the path as UTF-8 without BOM and without a trailing newline. The
   PowerShell wrapper must read it with `-Encoding UTF8` (Windows PowerShell 5.1 otherwise assumes ANSI):
   `$p = Get-Content $f -Raw -Encoding UTF8`.
7. **Default-branch fallback.** `git remote set-head origin --auto` runs only inside the background fetch phase,
   never during the instant local render. Until then, rows without `origin/HEAD` show `?`.
8. **Sorting.** Sort once after the initial local load and again on `r`. No live re-sort while fetch or actions
   update rows. The cursor tracks the row by path, not by index.
9. **Stale hints.** Merged and behind hints show from local refs at once, then update when the fetch finishes.
   They are not suppressed.

## Left to the builder

- Package layout. Keep it small: for example `main.go`, `git.go` (all git calls and parsing), `ui.go`
  (model/update/view). Split further only if a file gets unwieldy.
- Concurrency mechanics (a semaphore around git calls, delivered as `tea.Cmd` messages).
- Exact column widths and colors within the spirit above.
- Tests: table-driven tests for the parsers (porcelain status, `rev-list --left-right --count`,
  `worktree list --porcelain`, `symbolic-ref` output), plus one integration test that builds a temp repo with
  a bare remote and a worktree, and checks the state read and back-to-default. TUI rendering needs no tests.
