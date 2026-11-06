package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCDFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "cd")
	path := filepath.Join(t.TempDir(), "Ünï cödé", "my repo")
	if err := writeCDFile(f, path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(path)) {
		t.Fatalf("got %q want %q", got, path)
	}
}
