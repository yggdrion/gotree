package main

import "fmt"

// shellInit returns a wrapper function named gt for the given shell. A program
// cannot change its parent shell's directory, so the wrapper reads the path
// gotree writes to GOTREE_CD_FILE and cds there itself.
func shellInit(shell string) (string, error) {
	switch shell {
	case "bash", "zsh":
		return `gt() {
  local f p
  f=$(mktemp) || return
  GOTREE_CD_FILE=$f command gotree "$@"
  p=$(cat "$f")
  rm -f "$f"
  [ -n "$p" ] && cd -- "$p"
}
`, nil
	case "fish":
		return `function gt
    set -l f (mktemp); or return
    GOTREE_CD_FILE=$f command gotree $argv
    set -l p (cat $f)
    rm -f $f
    test -n "$p"; and cd -- $p
end
`, nil
	case "powershell", "pwsh":
		return `function gt { $f = New-TemporaryFile; $env:GOTREE_CD_FILE = $f; gotree.exe @args; $p = Get-Content $f -Raw -Encoding UTF8; Remove-Item $f; $env:GOTREE_CD_FILE = $null; if ($p) { Set-Location $p.Trim() } }
`, nil
	}
	return "", fmt.Errorf("unknown shell %q (want bash, zsh, fish or powershell)", shell)
}
