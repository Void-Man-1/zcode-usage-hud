package main

// Theme: every painted surface color is configurable. Defaults byte-match
// the historical grey schema; "Return to the void" restores them.
// Layout: theme.go owns colors only — geometry and animation live elsewhere.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

// colorVec is a blendable COLORREF (0x00BBGGRR layout, matching rgb()).
type colorVec struct{ v uint32 }

func cv(c uint32) colorVec { return colorVec{c} }

func (a colorVec) blend(b colorVec, k float64) colorVec {
	if k <= 0 {
		return a
	}
	if k >= 1 {
		return b
	}
	mix := func(x, y uint32) uint32 {
		return uint32(float64(x) + (float64(y)-float64(x))*k)
	}
	return colorVec{mix(a.v&0xFF, b.v&0xFF) | mix((a.v>>8)&0xFF, (b.v>>8)&0xFF)<<8 | mix((a.v>>16)&0xFF, (b.v>>16)&0xFF)<<16}
}

// themeKeys names every configurable element, in settings-dialog order.
var themeKeys = []string{
	"windowBg", "panel", "panelAlt", "border", "divider",
	"text", "textMuted", "textBright", "title",
	"accent", "ok", "warn", "bad",
	"barTrack", "barFill", "barFillWarn", "barFillBad",
	"gaugeArc", "gaugeNeedle", "standbyBg", "standbyText",
}

// themeDefaults is the original grey schema, byte-identical to the colors
// the HUD shipped with before theming existed.
var themeDefaults = map[string]uint32{
	"windowBg":    rgb(13, 14, 17),
	"panel":       rgb(24, 24, 29),
	"panelAlt":    rgb(27, 29, 35),
	"border":      rgb(42, 45, 54),
	"divider":     rgb(34, 36, 43),
	"text":        rgb(207, 207, 214),
	"textMuted":   rgb(125, 125, 137),
	"textBright":  rgb(247, 248, 250),
	"title":       rgb(210, 210, 219),
	"accent":      rgb(96, 165, 250),
	"ok":          rgb(91, 201, 128),
	"warn":        rgb(232, 167, 76),
	"bad":         rgb(196, 57, 67),
	"barTrack":    rgb(38, 40, 48),
	"barFill":     rgb(90, 190, 121),
	"barFillWarn": rgb(232, 167, 76),
	"barFillBad":  rgb(205, 75, 75),
	"gaugeArc":    rgb(96, 165, 250),
	"gaugeNeedle": rgb(247, 248, 250),
	"standbyBg":   rgb(24, 16, 18),
	"standbyText": rgb(196, 57, 67),
}

type themeState struct {
	mu       sync.RWMutex
	overridn map[string]uint32
}

var theme themeState

// themePath -> <appdata>/zcode-usage-hud/theme.json
func themePath() string { return filepath.Join(appDataDir(), "theme.json") }

// c returns the live color for an element: user override or default.
// Safe on any thread; painting calls it per frame.
func c(key string) colorVec {
	theme.mu.RLock()
	v, ok := theme.overridn[key]
	theme.mu.RUnlock()
	if !ok {
		v = themeDefaults[key]
	}
	return cv(v)
}

// setThemeColor applies one override live and persists the whole set.
func setThemeColor(key string, v uint32) {
	theme.mu.Lock()
	if theme.overridn == nil {
		theme.overridn = map[string]uint32{}
	}
	theme.overridn[key] = v
	theme.mu.Unlock()
	saveTheme()
}

// resetTheme clears every override ("Return to the void") and persists.
func resetTheme() {
	theme.mu.Lock()
	theme.overridn = map[string]uint32{}
	theme.mu.Unlock()
	saveTheme()
}

// themeOverrides returns a copy of the active overrides (for the dialog).
func themeOverrides() map[string]uint32 {
	theme.mu.RLock()
	defer theme.mu.RUnlock()
	out := make(map[string]uint32, len(theme.overridn))
	for k, v := range theme.overridn {
		out[k] = v
	}
	return out
}

// loadTheme restores saved overrides. Missing file, unknown keys, zero
// values, or malformed JSON leave the default palette untouched.
func loadTheme() {
	data, err := os.ReadFile(themePath())
	if err != nil {
		return
	}
	var saved map[string]uint32
	if json.Unmarshal(data, &saved) != nil || len(saved) == 0 {
		return
	}
	valid := map[string]uint32{}
	for _, k := range themeKeys {
		if v, ok := saved[k]; ok && v != 0 {
			valid[k] = v
		}
	}
	if len(valid) == 0 {
		return
	}
	theme.mu.Lock()
	theme.overridn = valid
	theme.mu.Unlock()
}

// saveTheme persists the current overrides atomically.
func saveTheme() {
	theme.mu.RLock()
	data, _ := json.Marshal(theme.overridn)
	theme.mu.RUnlock()
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		return
	}
	tmp := themePath() + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		os.Rename(tmp, themePath())
	}
}

// pickColorFor opens the standard color dialog seeded with `cur` and
// returns the chosen COLORREF plus whether the user confirmed.
func pickColorFor(owner uintptr, cur uint32) (uint32, bool) {
	var cc chooseColor
	cc.lStructSize = uint32(unsafe.Sizeof(cc))
	cc.hwndOwner = owner
	cc.rgbResult = cur
	cc.lpCustColors = &customColors[0]
	cc.flags = CC_FULLOPEN | CC_RGBINIT
	r, _, _ := procChooseColorW.Call(uintptr(unsafe.Pointer(&cc)))
	if r == 0 {
		return 0, false
	}
	return cc.rgbResult, true
}
