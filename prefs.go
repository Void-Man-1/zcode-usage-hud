package main

// Settings persistence: one settings.json holding animation speed, display
// style, bar geometry, home position, notification toggle, and refresh
// interval. Load/save are unit-tested via the LOCALAPPDATA seam.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// displayStyle selects how usage percentages render.
type displayStyle int

const (
	styleBars  displayStyle = iota // stacked horizontal bars (original)
	styleGauge                     // speedometer gauges in one row
)

func (d displayStyle) valid() bool { return d == styleBars || d == styleGauge }

// settings is the persisted configuration. Zero value = defaults; access
// goes through cfg getters so every read is clamped.
type settings struct {
	AnimMs        int32  `json:"animMs"` // window transition duration
	Style         int    `json:"style"`  // displayStyle
	BarW          int32  `json:"barW"`   // minimized bar width
	BarH          int32  `json:"barH"`   // minimized bar height
	HomeX         *int32 `json:"homeX"`  // nil = auto corner placement
	HomeY         *int32 `json:"homeY"`
	Notifications bool   `json:"notifications"` // tray toasts enabled
	RefreshSecs   int    `json:"refreshSecs"`   // data poll cadence
}

var (
	settingsMu  sync.RWMutex
	activePrefs = defaultSettings()
)

// defaultSettings mirrors the pre-settings behavior exactly.
func defaultSettings() settings {
	return settings{
		AnimMs:        160,
		Style:         int(styleBars),
		BarW:          0, // 0 = auto (notify-area 2/3, clamped 200..340)
		BarH:          0, // 0 = auto (taskbar height)
		Notifications: true,
		RefreshSecs:   5,
	}
}

func settingsPath() string { return filepath.Join(appDataDir(), "settings.json") }

// loadSettings restores saved settings. Malformed JSON, unknown style, or
// out-of-range values fall back to defaults per field.
func loadSettings() {
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		settingsMu.Lock()
		activePrefs = defaultSettings()
		settingsMu.Unlock()
		return
	}
	var s settings
	if json.Unmarshal(data, &s) != nil {
		settingsMu.Lock()
		activePrefs = defaultSettings()
		settingsMu.Unlock()
		return
	}
	def := defaultSettings()
	if !displayStyle(s.Style).valid() {
		s.Style = def.Style
	}
	s.AnimMs = clampI32(s.AnimMs, 80, 500)
	s.BarW = clampI32(s.BarW, 0, 600)
	s.BarH = clampI32(s.BarH, 0, 120)
	if s.HomeX != nil {
		hx := clampI32(*s.HomeX, -32000, 32000)
		s.HomeX = &hx
	}
	if s.HomeY != nil {
		hy := clampI32(*s.HomeY, -32000, 32000)
		s.HomeY = &hy
	}
	if s.RefreshSecs < 2 || s.RefreshSecs > 3600 {
		s.RefreshSecs = def.RefreshSecs
	}
	settingsMu.Lock()
	activePrefs = s
	settingsMu.Unlock()
}

// saveSettings writes the active settings atomically.
func saveSettings() {
	settingsMu.RLock()
	data, _ := json.Marshal(activePrefs)
	settingsMu.RUnlock()
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		return
	}
	tmp := settingsPath() + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		os.Rename(tmp, settingsPath())
	}
}

// storeSettings applies a full settings object and persists it.
func storeSettings(s settings) {
	def := defaultSettings()
	if !displayStyle(s.Style).valid() {
		s.Style = def.Style
	}
	s.AnimMs = clampI32(s.AnimMs, 80, 500)
	s.BarW = clampI32(s.BarW, 0, 600)
	s.BarH = clampI32(s.BarH, 0, 120)
	if s.RefreshSecs < 2 || s.RefreshSecs > 3600 {
		s.RefreshSecs = def.RefreshSecs
	}
	settingsMu.Lock()
	activePrefs = s
	settingsMu.Unlock()
	saveSettings()
}

// getSettings returns a snapshot of the active settings.
func getSettings() settings {
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return activePrefs
}

// --- clamped accessors (single ownership of derived limits) ---

func animDurationMs() int32 {
	s := getSettings()
	return clampI32(s.AnimMs, 80, 500)
}

func currentStyle() displayStyle { return displayStyle(getSettings().Style) }

func notifEnabled() bool { return getSettings().Notifications }

func refreshInterval() time.Duration {
	return time.Duration(getSettings().RefreshSecs) * time.Second
}

// barSize resolves the minimized bar dimensions: user override when set,
// otherwise the auto behavior (notify-area 2/3 width, taskbar height).
func barSize(autoW, autoH int32) (int32, int32) {
	s := getSettings()
	w, h := autoW, autoH
	if s.BarW >= 120 {
		w = s.BarW
	}
	if s.BarH >= 24 {
		h = s.BarH
	}
	return w, h
}

// homePosition returns the stored snap target, or false when unset.
func homePosition() (int32, int32, bool) {
	s := getSettings()
	if s.HomeX == nil || s.HomeY == nil {
		return 0, 0, false
	}
	return *s.HomeX, *s.HomeY, true
}

// setHomePosition stores the bar's top-left as the snap target.
func setHomePosition(x, y int32) {
	settingsMu.Lock()
	activePrefs.HomeX = &x
	activePrefs.HomeY = &y
	settingsMu.Unlock()
	saveSettings()
}

func clampI32(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
