package config

import (
	"os"
	"testing"
	"time"
)

// Isolate runs from the real config dir on every OS.
func isolate(t *testing.T) {
	d := t.TempDir()
	t.Setenv("AppData", d)
	t.Setenv("XDG_CONFIG_HOME", d)
	t.Setenv("HOME", d)
}

func TestLoadMissingIsUnanswered(t *testing.T) {
	isolate(t)
	if c := Load(); c.Autofetch != nil {
		t.Fatalf("want nil Autofetch, got %v", *c.Autofetch)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	isolate(t)
	on := true
	if err := Save(Config{Autofetch: &on, Minutes: 10}); err != nil {
		t.Fatal(err)
	}
	c := Load()
	if c.Autofetch == nil || !*c.Autofetch || c.Interval() != 10*time.Minute {
		t.Fatalf("got %+v", c)
	}
}

func TestBadIntervalAndCorruptFile(t *testing.T) {
	isolate(t)
	if (Config{Minutes: -3}).Interval() != 5*time.Minute {
		t.Fatal("bad interval should default to 5m")
	}
	p, _ := Path()
	if err := os.MkdirAll(p[:len(p)-len("config.json")], 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if Load().Autofetch != nil {
		t.Fatal("corrupt file should load as unanswered")
	}
}
