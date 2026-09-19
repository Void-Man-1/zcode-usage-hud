package main

import (
	"os"
	"testing"
	"time"
)

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// Drives the real save/load through the LOCALAPPDATA seam (t.Setenv
// auto-restores it), including corrupt-file and zero-value fallbacks.
func TestThemeRoundTrip(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())

	// Defaults active before any override.
	if got := c("windowBg").v; got != themeDefaults["windowBg"] {
		t.Fatalf("default windowBg = %x, want %x", got, themeDefaults["windowBg"])
	}

	setThemeColor("windowBg", 0x101010)
	setThemeColor("accent", 0x203040)
	if got := c("accent").v; got != 0x203040 {
		t.Fatalf("accent after set = %x, want 203040", got)
	}

	// Simulate restart: fresh state, reload from disk.
	theme.mu.Lock()
	theme.overridn = nil
	theme.mu.Unlock()
	loadTheme()
	if got := c("windowBg").v; got != 0x101010 {
		t.Fatalf("windowBg after reload = %x, want 101010", got)
	}
	if got := c("accent").v; got != 0x203040 {
		t.Fatalf("accent after reload = %x, want 203040", got)
	}

	// Return to the void: overrides cleared and persisted.
	resetTheme()
	theme.mu.Lock()
	theme.overridn = nil
	theme.mu.Unlock()
	loadTheme()
	if got := c("windowBg").v; got != themeDefaults["windowBg"] {
		t.Fatalf("after void: windowBg = %x, want default %x", got, themeDefaults["windowBg"])
	}
}

func TestThemeCorruptFileFallsBack(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(themePath(), []byte("{not json")); err != nil {
		t.Fatal(err)
	}
	theme.mu.Lock()
	theme.overridn = nil
	theme.mu.Unlock()
	loadTheme()
	if got := c("panel").v; got != themeDefaults["panel"] {
		t.Fatalf("corrupt theme: panel = %x, want default", got)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	loadSettings()
	s := getSettings()
	if s.AnimMs != 160 || s.RefreshSecs != 5 || !s.Notifications {
		t.Fatalf("defaults wrong: %+v", s)
	}

	s.AnimMs = 300
	s.Style = int(styleGauge)
	s.BarW = 280
	s.BarH = 56
	hx, hy := int32(1234), int32(567)
	s.HomeX, s.HomeY = &hx, &hy
	s.Notifications = false
	s.RefreshSecs = 9
	storeSettings(s)

	// Restart simulation.
	settingsMu.Lock()
	activePrefs = defaultSettings()
	settingsMu.Unlock()
	loadSettings()
	got := getSettings()
	if got.AnimMs != 300 || got.Style != int(styleGauge) || got.BarW != 280 || got.BarH != 56 {
		t.Fatalf("settings after reload = %+v", got)
	}
	if got.HomeX == nil || *got.HomeX != 1234 || got.HomeY == nil || *got.HomeY != 567 {
		t.Fatalf("home after reload = %+v", got)
	}
	if got.Notifications {
		t.Fatal("notifications should persist off")
	}
	if got.RefreshSecs != 9 {
		t.Fatalf("refresh = %d, want 9", got.RefreshSecs)
	}
}

func TestSettingsClamps(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	storeSettings(settings{AnimMs: 10, Style: 99, BarW: 9999, BarH: 999, RefreshSecs: 0})
	got := getSettings()
	if got.AnimMs != 80 {
		t.Fatalf("anim clamp: %d", got.AnimMs)
	}
	if got.Style != int(styleBars) {
		t.Fatalf("bad style fell back to bars: %d", got.Style)
	}
	if got.BarW != 600 || got.BarH != 120 {
		t.Fatalf("bar clamps: %d,%d", got.BarW, got.BarH)
	}
	if got.RefreshSecs != 5 {
		t.Fatalf("refresh clamp: %d", got.RefreshSecs)
	}
}

func TestParseSemver(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
		ok   bool
	}{{"v1.2.3", [3]int{1, 2, 3}, true}, {"1.2", [3]int{1, 2, 0}, true},
		{"v10.0.11", [3]int{10, 0, 11}, true}, {"v1", [3]int{}, false},
		{"vx.2.3", [3]int{}, false}, {"", [3]int{}, false}}
	for _, tc := range cases {
		got, ok := parseSemver(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parseSemver(%q) = %v,%v want %v,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestIsNewerVersion(t *testing.T) {
	if !isNewerVersion("1.2.1", "v1.2.2") {
		t.Error("patch bump should be newer")
	}
	if !isNewerVersion("1.2.1", "v1.3.0") {
		t.Error("minor bump should be newer")
	}
	if !isNewerVersion("1.2.1", "v2.0.0") {
		t.Error("major bump should be newer")
	}
	if isNewerVersion("1.2.1", "v1.2.1") {
		t.Error("same version is not newer")
	}
	if isNewerVersion("1.2.1", "v1.2.0") {
		t.Error("older is not newer")
	}
	if isNewerVersion("1.2.1", "not-a-version") {
		t.Error("garbage is not newer")
	}
}

func TestStandbyStripText(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	alive := Balance{Period: "daily", Total: 100, Used: 50}
	dead := Balance{Period: "daily", Total: 100, Used: 100, PeriodStart: now.Add(-1 * time.Hour), PeriodEnd: now.Add(2 * time.Hour)}

	if got := standbyStripText([]Balance{alive}, now); got != "OUT OF USAGE" {
		t.Fatalf("no dead buckets: %q", got)
	}
	got := standbyStripText([]Balance{dead}, now)
	if got == "" || got == "OUT OF USAGE" {
		t.Fatalf("one dead bucket should render a countdown, got %q", got)
	}
	// Multiple exhausted buckets: both join into one line.
	got = standbyStripText([]Balance{dead, dead}, now)
	if len(got) <= len(standbyStripText([]Balance{dead}, now)) {
		t.Fatalf("two dead buckets should join, got %q", got)
	}
}

func TestEaseOutCubic(t *testing.T) {
	if easeOutCubic(0) != 0 || easeOutCubic(1) != 1 {
		t.Fatal("endpoints must be exact")
	}
	if easeOutCubic(0.25) >= easeOutCubic(0.75) {
		t.Fatal("must be increasing")
	}
	// Fast start: 25% time should give well over 25% progress.
	if easeOutCubic(0.25) < 0.55 {
		t.Fatalf("easeOut(0.25) = %v, want fast start", easeOutCubic(0.25))
	}
}

func TestGaugeLayout(t *testing.T) {
	cells := gaugeLayout(3, 0, 0, 300, 70)
	if len(cells) != 3 {
		t.Fatalf("want 3 cells, got %d", len(cells))
	}
	// One horizontal line: no vertical overlap, in order.
	for i := 1; i < len(cells); i++ {
		if cells[i].Left < cells[i-1].Right {
			t.Fatalf("cells overlap horizontally: %v then %v", cells[i-1], cells[i])
		}
	}
	if gaugeLayout(0, 0, 0, 100, 40) != nil {
		t.Fatal("zero buckets -> nil layout")
	}
}

func TestGaugeRowHeightFitsOneLine(t *testing.T) {
	if gaugeRowHeight() < 50 {
		t.Fatal("gauge row must leave room for the arc + label")
	}
}
