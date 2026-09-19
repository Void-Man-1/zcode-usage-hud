package main

// Settings window: one native window with clearly separated sections —
// Appearance, Animation, Layout, Behavior, Updates. Controls are painted
// with the live theme and click hit-tested via rects rebuilt on paint,
// matching the HUD's owner-drawn style.

import (
	"fmt"
	"syscall"
	"unsafe"
)

const settingsWndClass = "ZCodeHUDSettingsWnd"

// Control ids for hover/click hit-testing. Order = paint order.
const (
	scNone = iota
	scStyleBars
	scStyleGauge
	scVoid
	scAnimMinus
	scAnimPlus
	scBarWMinus
	scBarWPlus
	scBarHMinus
	scBarHPlus
	scSetHome
	scClearHome
	scNotif
	scRefreshMinus
	scRefreshPlus
	scCheckUpdates
	scAccount

	// Color swatches: one per themeKeys entry, id = scColorBase + index.
	// Must stay LAST: chip ids allocate upward from here and must never
	// collide with the fixed controls above. (They once started at 3 and
	// overwrote scVoid+; every control below the grid then opened a color
	// dialog instead of doing its job.)
	scColorBase
)

// colorKeyFor maps a swatch control id to its theme key.
func colorKeyFor(id int) (string, bool) {
	i := id - scColorBase
	if i < 0 || i >= len(themeKeys) {
		return "", false
	}
	return themeKeys[i], true
}

var (
	hwndSettings uintptr
	setHover     int
	setRects     map[int]RECT
	setFontH     uintptr
	setFontB     uintptr
	setFontS     uintptr
)

// openSettings creates (or focuses) the settings window. Every call site
// (gear click, tray menu) runs on the HUD's pumped UI thread, so the window
// is created inline there — a window needs the creating thread to pump
// messages, and goroutine threads have no pump.
func openSettings() {
	if hwndSettings != 0 {
		procShowWindow.Call(hwndSettings, SW_SHOW)
		procSetForegroundWindow.Call(hwndSettings)
		return
	}
	openSettingsOnUI()
}

func openSettingsOnUI() {
	cls, _ := syscall.UTF16PtrFromString(settingsWndClass)
	wc := WNDCLASSEX{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
		LpfnWndProc:   syscall.NewCallback(settingsWndProc),
		HInstance:     func() uintptr { r, _, _ := procGetModuleHandleW.Call(); return r }(),
		LpszClassName: cls,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	name, _ := syscall.UTF16PtrFromString("HUD Settings")
	h, _, _ := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(name)),
		uintptr(WS_POPUP|0x00C00000), // WS_POPUP|WS_CAPTION|WS_SYSMENU
		100, 100, 560, 1020, 0, 0, wc.HInstance, 0)
	if h == 0 {
		logDiagnostic("settings CreateWindowExW failed")
		return
	}
	hwndSettings = h
	logDiagnostic("settings window created")
	procShowWindow.Call(h, SW_SHOW)
}

func settingsWndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		hwndSettings = 0
		return 0
	case WM_PAINT:
		paintSettings(hwnd)
		return 0
	case WM_MOUSEMOVE:
		x, y := mouseXY(lParam)
		h := settingsHitTest(x, y)
		if h != setHover {
			setHover = h
			procInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case WM_LBUTTONUP:
		x, y := mouseXY(lParam)
		handleSettingsClick(hwnd, settingsHitTest(x, y))
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

func settingsHitTest(x, y int32) int {
	for id, rc := range setRects {
		if pointInRect(x, y, rc) {
			return id
		}
	}
	return scNone
}

func setRect(id int, rc RECT) { setRects[id] = rc }
func hoverGlow(id int) bool   { return setHover == id }

// sectionTitle paints one section header plus its divider line.
func sectionTitle(hdc uintptr, title string, y int32, w int32) int32 {
	drawText(hdc, setFontH, c("textBright").v, title, 20, y, w-20, y+22, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	fillPanel(hdc, RECT{20, y + 26, w - 20, y + 27}, c("divider").v)
	return y + 38
}

// swatchRow paints one color control: label + clickable swatch.
func swatchRow(hdc uintptr, id int, label string, key string, y int32) (int32, RECT) {
	rc := RECT{20, y, 240, y + 24}
	drawText(hdc, setFontB, c("text").v, label, rc.Left, rc.Top, rc.Right-70, rc.Bottom, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	swatch := RECT{240 - 24, y + 2, 240, y + 22}
	fillPanel(hdc, swatch, c(key).v)
	framePanel(hdc, swatch, c("border").v)
	if hoverGlow(id) {
		framePanel(hdc, swatch, c("textBright").v)
	}
	return y + 30, swatch
}

// stepperRow paints "label  [- value +]".
func stepperRow(hdc uintptr, minus, plus int, label string, val string, y int32) int32 {
	drawText(hdc, setFontB, c("text").v, label, 20, y, 300, y+24, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	m := RECT{300, y, 330, y + 24}
	p := RECT{388, y, 418, y + 24}
	v := RECT{330, y, 388, y + 24}
	for _, b := range []struct {
		rc RECT
		id int
		tx string
	}{{m, minus, "−"}, {p, plus, "+"}} {
		fillPanel(hdc, b.rc, c("panelAlt").v)
		framePanel(hdc, b.rc, c("border").v)
		if hoverGlow(b.id) {
			framePanel(hdc, b.rc, c("textBright").v)
		}
		drawText(hdc, setFontB, c("text").v, b.tx, b.rc.Left, b.rc.Top, b.rc.Right, b.rc.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		setRect(b.id, b.rc)
	}
	drawText(hdc, setFontB, c("textBright").v, val, v.Left, v.Top, v.Right, v.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	return y + 32
}

// toggleRow paints a checkbox-style toggle.
func toggleRow(hdc uintptr, id int, label string, on bool, y int32) int32 {
	box := RECT{20, y, 44, y + 24}
	setRect(id, box)
	fillPanel(hdc, box, c("panelAlt").v)
	framePanel(hdc, box, c("border").v)
	if on {
		fillPanel(hdc, RECT{box.Left + 5, box.Top + 5, box.Right - 5, box.Bottom - 5}, c("ok").v)
	}
	drawText(hdc, setFontB, c("text").v, label, 56, y, 500, y+24, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	if hoverGlow(id) {
		framePanel(hdc, box, c("textBright").v)
	}
	return y + 32
}

// buttonRow paints a wide action button.
func buttonRow(hdc uintptr, id int, label string, y int32, w int32, danger bool) int32 {
	rc := RECT{20, y, w - 20, y + 32}
	bg := c("panelAlt")
	if danger {
		bg = c("standbyBg")
	}
	fillPanel(hdc, rc, bg.v)
	framePanel(hdc, rc, c("border").v)
	if hoverGlow(id) {
		framePanel(hdc, rc, c("textBright").v)
	}
	col := c("textBright")
	if danger {
		col = c("bad")
	}
	drawText(hdc, setFontB, col.v, label, rc.Left, rc.Top, rc.Right, rc.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	setRect(id, rc)
	return y + 42
}

// paintSettings renders the whole dialog with the live theme.
func paintSettings(hwnd uintptr) {
	if setRects == nil {
		setRects = map[int]RECT{}
	}
	setRects = map[int]RECT{}
	if setFontH == 0 {
		setFontH = createFont(11, FW_SEMIBOLD)
		setFontB = createFont(9, FW_NORMAL)
		setFontS = createFont(8, FW_NORMAL)
	}
	var ps PAINTSTRUCT
	targetDC, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if targetDC == 0 {
		return
	}
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var rc RECT
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
	w := rc.Right - rc.Left

	bg := createBrush(c("windowBg").v)
	defer procDeleteObject.Call(bg)
	procFillRect.Call(targetDC, uintptr(unsafe.Pointer(&rc)), bg)
	procSetBkMode.Call(targetDC, TRANSPARENT)

	s := getSettings()
	y := sectionTitle(targetDC, "APPEARANCE", 14, w)

	// Style picker: two side-by-side buttons.
	barsRc := RECT{20, y, 270, y + 30}
	gaugeRc := RECT{290, y, w - 20, y + 30}
	styleBtn := func(r RECT, id int, label string, active bool) {
		fillPanel(targetDC, r, c("panelAlt").v)
		framePanel(targetDC, r, c("border").v)
		if active {
			framePanel(targetDC, r, c("accent").v)
		}
		if hoverGlow(id) {
			framePanel(targetDC, r, c("textBright").v)
		}
		drawText(targetDC, setFontB, c("text").v, label, r.Left, r.Top, r.Right, r.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		setRect(id, r)
	}
	styleBtn(barsRc, scStyleBars, "Horizontal bars", s.Style == int(styleBars))
	styleBtn(gaugeRc, scStyleGauge, "Speedometer gauges", s.Style == int(styleGauge))
	y += 40

	// Color grid: every theme key, 6 chips per row, name under each.
	// The hovered key's name repeats in a full-width status line so
	// truncated labels stay readable.
	const cols = 6
	cw, ch := int32(78), int32(22)
	gapX, gapY := int32(3), int32(16)
	hoverKey := ""
	for i, key := range themeKeys {
		row, col := int32(i/cols), int32(i%cols)
		rc := RECT{20 + col*(cw+gapX), y + row*(ch+gapY), 20 + col*(cw+gapX) + cw, y + row*(ch+gapY) + ch}
		fillPanel(targetDC, rc, c(key).v)
		framePanel(targetDC, rc, c("border").v)
		id := scColorBase + i
		if hoverGlow(id) {
			framePanel(targetDC, rc, c("textBright").v)
			hoverKey = key
		}
		drawText(targetDC, setFontS, c("textMuted").v, key, rc.Left, rc.Bottom+1, rc.Right, rc.Bottom+13, DT_CENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		setRect(id, rc)
	}
	rows := (int32(len(themeKeys)) + cols - 1) / cols
	y += rows*(ch+gapY) + 4
	if hoverKey != "" {
		drawText(targetDC, setFontB, c("text").v, hoverKey, 20, y, w-20, y+18, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	}
	y += 20
	y = buttonRow(targetDC, scVoid, "Return to the void (reset colors)", y, w, false)

	y = sectionTitle(targetDC, "ANIMATION", y, w)
	y = stepperRow(targetDC, scAnimMinus, scAnimPlus, "Transition speed", fmt.Sprintf("%d ms", s.AnimMs), y)
	drawText(targetDC, setFontS, c("textMuted").v, "Ease-in-out applied to minimize/expand, mode switch and standby.", 20, y, w-20, y+18, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	y += 24

	y = sectionTitle(targetDC, "MINIMIZED BAR", y, w)
	y = stepperRow(targetDC, scBarWMinus, scBarWPlus, "Bar width", fmt.Sprintf("%d px", s.BarW), y)
	y = stepperRow(targetDC, scBarHMinus, scBarHPlus, "Bar height", fmt.Sprintf("%d px", s.BarH), y)
	homeX, homeY, has := homePosition()
	homeLabel := "Set home to bar position"
	if has {
		homeLabel = fmt.Sprintf("Set home (current: %d,%d)", homeX, homeY)
	}
	y = buttonRow(targetDC, scSetHome, homeLabel, y, w, false)
	y = buttonRow(targetDC, scClearHome, "Clear home (auto corner)", y, w, false)
	drawText(targetDC, setFontS, c("textMuted").v, "The bar snaps home every time it minimizes.", 20, y, w-20, y+18, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
	y += 24

	y = sectionTitle(targetDC, "BEHAVIOR", y, w)
	y = toggleRow(targetDC, scNotif, "Windows notifications", s.Notifications, y)
	y = stepperRow(targetDC, scRefreshMinus, scRefreshPlus, "Data refresh", fmt.Sprintf("%d s", s.RefreshSecs), y)

	y = sectionTitle(targetDC, "UPDATES & ACCOUNT", y, w)
	y = buttonRow(targetDC, scCheckUpdates, "Check for updates", y, w, false)
	y = buttonRow(targetDC, scAccount, "Manage account (tray menu)", y, w, false)
	drawText(targetDC, setFontS, c("textMuted").v,
		"v"+appVersion+" — updates download and install silently.", 20, y, w-20, y+18, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
}

// handleSettingsClick applies one control's action.
func handleSettingsClick(hwnd uintptr, id int) {
	s := getSettings()
	// Color swatches: one id per themeKeys entry.
	if key, ok := colorKeyFor(id); ok {
		if v, ok := pickColorFor(hwnd, c(key).v); ok {
			setThemeColor(key, v)
			repaintAll()
		}
		procInvalidateRect.Call(hwnd, 0, 0)
		return
	}
	switch id {
	case scStyleBars, scStyleGauge:
		want := styleBars
		if id == scStyleGauge {
			want = styleGauge
		}
		if displayStyle(s.Style) != want {
			toggleDisplayStyle(want)
		}
	case scVoid:
		resetTheme()
		repaintAll()
	case scAnimMinus:
		storeSettings(settings{AnimMs: clampI32(s.AnimMs-20, 80, 500), Style: s.Style, BarW: s.BarW, BarH: s.BarH, HomeX: s.HomeX, HomeY: s.HomeY, Notifications: s.Notifications, RefreshSecs: s.RefreshSecs})
	case scAnimPlus:
		storeSettings(settings{AnimMs: clampI32(s.AnimMs+20, 80, 500), Style: s.Style, BarW: s.BarW, BarH: s.BarH, HomeX: s.HomeX, HomeY: s.HomeY, Notifications: s.Notifications, RefreshSecs: s.RefreshSecs})
	case scBarWMinus:
		storeSettings(clampBar(s, s.BarW-20, s.BarH))
	case scBarWPlus:
		storeSettings(clampBar(s, s.BarW+20, s.BarH))
	case scBarHMinus:
		storeSettings(clampBar(s, s.BarW, s.BarH-4))
	case scBarHPlus:
		storeSettings(clampBar(s, s.BarW, s.BarH+4))
	case scSetHome:
		if hwndMain != 0 {
			// Home = the HUD bar's top-left, not the settings window's.
			r := windowRect(hwndMain)
			setHomePosition(r.Left, r.Top)
		}
	case scClearHome:
		settingsMu.Lock()
		activePrefs.HomeX, activePrefs.HomeY = nil, nil
		settingsMu.Unlock()
		saveSettings()
	case scNotif:
		storeSettings(toggleNotif(s))
	case scRefreshMinus:
		storeSettings(setRefresh(s, s.RefreshSecs-1))
	case scRefreshPlus:
		storeSettings(setRefresh(s, s.RefreshSecs+1))
	case scCheckUpdates:
		go checkForUpdatesInteractive(hwnd)
	case scAccount:
		if hwndMain != 0 {
			showMenu(hwndMain)
		}
	}
	procInvalidateRect.Call(hwnd, 0, 0)
	if id >= scStyleBars && id <= scStyleGauge && hwndMain != 0 {
		procInvalidateRect.Call(hwndMain, 0, 0)
	}
}

// clampBar applies bar-size clamps to a candidate setting.
func clampBar(s settings, w, h int32) settings {
	s.BarW = clampI32(w, 0, 600)
	s.BarH = clampI32(h, 0, 120)
	return s
}

func toggleNotif(s settings) settings {
	s.Notifications = !s.Notifications
	return s
}

func setRefresh(s settings, v int) settings {
	if v < 2 {
		v = 2
	}
	if v > 3600 {
		v = 3600
	}
	s.RefreshSecs = v
	return s
}

// repaintAll invalidates both windows and re-applies live timers.
func repaintAll() {
	if hwndMain != 0 {
		procInvalidateRect.Call(hwndMain, 0, 0)
		applyRefreshTimer(hwndMain)
	}
	if hwndSettings != 0 {
		procInvalidateRect.Call(hwndSettings, 0, 0)
	}
}

// toggleDisplayStyle switches rendering mode with an animated morph.
func toggleDisplayStyle(want displayStyle) {
	s := getSettings()
	if displayStyle(s.Style) == want {
		return
	}
	s.Style = int(want)
	storeSettings(s)
	if hwndMain == 0 {
		return
	}
	if collapsed {
		// Bar re-geometry: eased from current rect to the new mode's.
		fromR := windowRect(hwndMain)
		x, y, w, h, _ := collapsedGeometryEx()
		startStyleMorph(fromR, RECT{Left: x, Top: y, Right: x + w, Bottom: y + h})
	} else {
		fromR := windowRect(hwndMain)
		startStyleMorph(fromR, RECT{Left: fromR.Left, Top: fromR.Top,
			Right: fromR.Left + winWidth, Bottom: fromR.Top + winHeight})
	}
	repaintAll()
}
