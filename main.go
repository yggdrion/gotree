package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yggdrion/gotree/internal/ui"
)

// version is set by ldflags: -X main.version=...
var version = "dev"

func writeCDFile(file, path string) error {
	return os.WriteFile(file, []byte(path), 0o600)
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	initShell := flag.String("init", "", "print the gt shell wrapper for `shell` (bash, zsh, fish, powershell) and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("gotree " + version)
		return
	}
	if *initShell != "" {
		s, err := shellInit(*initShell)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gotree:", err)
			os.Exit(2)
		}
		fmt.Print(s)
		return
	}

	root := flag.Arg(0)
	if root == "" {
		var err error
		if root, err = os.Getwd(); err != nil {
			fmt.Fprintln(os.Stderr, "gotree:", err)
			os.Exit(1)
		}
	}
	root, err := filepath.Abs(root)
	if err == nil {
		var fi os.FileInfo
		if fi, err = os.Stat(root); err == nil && !fi.IsDir() {
			err = fmt.Errorf("%s is not a directory", root)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gotree:", err)
		os.Exit(1)
	}

	chosen, err := ui.Run(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gotree:", err)
		os.Exit(1)
	}
	if chosen == "" {
		return
	}
	if f := os.Getenv("GOTREE_CD_FILE"); f != "" {
		if err := writeCDFile(f, chosen); err != nil {
			fmt.Fprintln(os.Stderr, "gotree:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Print(chosen)
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		// Printed to a terminal, so no wrapper is capturing it: say how to set one up.
		fmt.Fprintln(os.Stderr, "\ngotree: cannot cd the calling shell by itself. Add this to your shell rc and use gt:")
		fmt.Fprintln(os.Stderr, `  eval "$(gotree --init bash)"   # or zsh / fish (gotree --init fish | source) / powershell`)
	}
}
