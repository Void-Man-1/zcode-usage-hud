package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAccentPrefRoundTrip(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	os.Remove(accentPrefPath())

	// Save a picked accent, reload it, expect the same value back.
	accentSet, accentValue = true, 0x00384F5A // COLORREF 0x00BBGGRR
	saveAccentPref()
	accentSet, accentValue = false, 0
	loadAccentPref()
	if !accentSet || accentValue != 0x00384F5A {
		t.Fatalf("round trip: set=%t value=%#x, want true 0x384f5a", accentSet, accentValue)
	}

	// Missing file must leave the default palette untouched.
	accentSet, accentValue = false, 0
	if err := os.Remove(accentPrefPath()); err != nil {
		t.Fatalf("remove pref: %v", err)
	}
	loadAccentPref()
	if accentSet || accentValue != 0 {
		t.Fatalf("missing file: set=%t value=%#x, want false 0", accentSet, accentValue)
	}
}

func TestAccentPrefCorruptFile(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(accentPrefPath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	accentSet, accentValue = false, 0
	loadAccentPref()
	if accentSet || accentValue != 0 {
		t.Fatalf("corrupt file: set=%t value=%#x, want false 0", accentSet, accentValue)
	}
	// A zero accent (user picked pure black slot crash) must also be ignored.
	if err := os.WriteFile(accentPrefPath(), []byte(`{"accent":0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loadAccentPref()
	if accentSet {
		t.Fatal("zero accent must be rejected, keeping the default palette")
	}
}

func TestAccentPrefPathUnderAppData(t *testing.T) {
	t.Setenv("LOCALAPPDATA", filepath.Join("some", "where"))
	if got, want := accentPrefPath(), filepath.Join("some", "where", "ZCode Usage HUD", "accent.json"); got != want {
		t.Fatalf("accentPrefPath = %q, want %q", got, want)
	}
}
