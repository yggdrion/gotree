# gotree

A terminal UI for managing many git repos at once. It lists every repo in a folder with branch, ahead/behind, dirty state and worktrees, and lets you pull, fetch, reset to the default branch, open in VS Code, or `cd` into one.

## Install

- macOS: `brew install yggdrion/tap/gotree`
- Windows and Linux: download the archive for your platform from the [releases page](https://github.com/yggdrion/gotree/releases).
- With Go: `go install github.com/yggdrion/gotree@latest`

## Shell integration

A program cannot change its parent shell's directory, so `x` (cd) needs a small wrapper function `gt`. It sets `GOTREE_CD_FILE`, runs gotree, and `cd`s to the path gotree wrote there. gotree prints the wrapper for you:

```bash
eval "$(gotree --init bash)"      # ~/.bashrc
eval "$(gotree --init zsh)"       # ~/.zshrc
gotree --init fish | source       # ~/.config/fish/config.fish
```

Or paste it by hand:

PowerShell (put in `$PROFILE`):

```powershell
function gt { $f = New-TemporaryFile; $env:GOTREE_CD_FILE = $f; gotree.exe @args; $p = Get-Content $f -Raw -Encoding UTF8; Remove-Item $f; $env:GOTREE_CD_FILE = $null; if ($p) { Set-Location $p.Trim() } }
```

bash/zsh (put in `~/.bashrc` or `~/.zshrc`):

```bash
gt() { local f; f=$(mktemp); GOTREE_CD_FILE=$f gotree "$@"; local p; p=$(cat "$f"); rm -f "$f"; [ -n "$p" ] && cd "$p"; }
```

Without the wrapper, gotree only prints the chosen path (plus a setup hint on stderr when stdout is a terminal).

## Usage

```
gt            # repos in the current folder
gt <dir>      # repos in <dir>
go run . <dir>  # from a checkout/worktree
```

## Keys

| Key | Action |
| --- | --- |
| `↑`/`↓`, `j`/`k` | move |
| `enter` | action menu with repo state |
| `x` | cd into the folder |
| `c` | open in VS Code |
| `b` | back to default branch + pull |
| `p` | pull (`--ff-only`) |
| `P` | pull all repos that are clean, behind and not ahead |
| `f` | fetch all |
| `r` | refresh |
| `space` | select |
| `→`/`←` | fold / unfold worktrees |
| `d` | delete worktree |
| `/` | filter |
| `?` | help |
| `q` | quit |
