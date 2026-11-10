package main

import (
	"strings"
	"testing"
)

func TestShellInit(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", "fish", "powershell", "pwsh"} {
		s, err := shellInit(sh)
		if err != nil {
			t.Fatalf("%s: %v", sh, err)
		}
		if !strings.Contains(s, "GOTREE_CD_FILE") {
			t.Fatalf("%s: wrapper does not set GOTREE_CD_FILE", sh)
		}
	}
	if _, err := shellInit("tcsh"); err == nil {
		t.Fatal("want error for unknown shell")
	}
}
