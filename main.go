package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// version is set by ldflags: -X main.version=...
var version = "dev"

func writeCDFile(file, path string) error {
	return os.WriteFile(file, []byte(path), 0o600)
}

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("gotree " + version)
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

	final, err := tea.NewProgram(newModel(root), tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gotree:", err)
		os.Exit(1)
	}
	m, ok := final.(model)
	if !ok || m.Chosen() == "" {
		return
	}
	if f := os.Getenv("GOTREE_CD_FILE"); f != "" {
		if err := writeCDFile(f, m.Chosen()); err != nil {
			fmt.Fprintln(os.Stderr, "gotree:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Print(m.Chosen())
}
