package main

// Animation engine: one timer-driven state machine owning every window
// transition. UI-thread only. Kinds:
//
//	rect       — eased window-rect move/resize (collapse/expand/strip)
//	styleMorph — bars<->gauges crossfade, rect interpolates in parallel
//	stdFadeOut — gauges fade out (ease-out) before the outage alarm
//	stdFlash   — background flashes red 5x while "OUT OF USAGE" fades in
//	stdShrink  — eased collapse into the standby strip
//	stdRestore — eased reopen once buckets refill

import (
	"time"
)

const (
	animFPS       = 16 * time.Millisecond
	flashCount    = 5
	flashCycle    = 1100 * time.Millisecond
	flashOn       = 550 * time.Millisecond
	stdFadeoutMs  = 260
	stdFlashMs    = 5500 // 5 x 1100ms
	stdTextFadeMs = 700
)

type hudAnim struct {
	phase   string
	start   time.Time
	visible bool
	fromR   RECT
	toR     RECT
}

// flashState maps elapsed flash time to (visible, all blinks done).
func flashState(elapsed time.Duration) (visible, done bool) {
	cycles := int(elapsed / flashCycle)
	if cycles >= flashCount {
		return false, true
	}
	return elapsed%flashCycle < flashOn, false
}

// easeInOutCubic: slow-fast-slow interpolation, 0..1. Pure - unit-tested.
func easeInOutCubic(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := 2*t - 2
	return 1 + u*u*u/2
}

// easeOutCubic: fast start, gentle settle. Pure - unit-tested.
func easeOutCubic(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	u := 1 - t
	return 1 - u*u*u
}

// animElapsedMs is the eased progress 0..1 for an animation started at
// a.start with the user-configured duration. Pure enough for tests via
// injectable duration.
func animProgress(a *hudAnim, durMs int32) float64 {
	t := float64(time.Since(a.start).Milliseconds()) / float64(durMs)
	if t > 1 {
		t = 1
	}
	return easeInOutCubic(t)
}

func startAnimTimer() {
	procSetTimer.Call(hwndMain, TIMER_ANIM, uintptr(animFPS/time.Millisecond), 0)
}

func stopAnimTimer() {
	procKillTimer.Call(hwndMain, TIMER_ANIM)
}

// startRectTransition eases the window between two rects. UI-thread only.
func startRectTransition(fromR, toR RECT) {
	if sameRect(fromR, toR) {
		return
	}
	anim = hudAnim{phase: "rect", start: time.Now(), fromR: fromR, toR: toR}
	startAnimTimer()
}

// startStyleMorph runs the bars<->gauges crossfade while the window rect
// eases from its current size to the target mode's size.
func startStyleMorph(fromR, toR RECT) {
	anim = hudAnim{phase: "styleMorph", start: time.Now(), fromR: fromR, toR: toR}
	startAnimTimer()
}

// startStandbyFadeOut begins the outage sequence: content fades to zero
// alpha over stdFadeoutMs before the flash starts.
func startStandbyFadeOut() {
	anim = hudAnim{phase: "stdFadeOut", start: time.Now()}
	startAnimTimer()
}

// startStandbyFlash runs the 5x red background flash; the OUT OF USAGE
// text fades in over the first beat and stays.
func startStandbyFlash() {
	anim = hudAnim{phase: "stdFlash", start: time.Now(), visible: true}
	startAnimTimer()
}

// startStandbyShrink eases the window into the standby strip rect.
func startStandbyShrink(fromR, toR RECT) {
	anim = hudAnim{phase: "stdShrink", start: time.Now(), fromR: fromR, toR: toR}
	startAnimTimer()
}

// startStandbyRestore eases the bar back open to the normal bar geometry.
func startStandbyRestore(fromR, toR RECT) {
	anim = hudAnim{phase: "stdRestore", start: time.Now(), fromR: fromR, toR: toR}
	startAnimTimer()
}

// stepAnimation advances the active animation by one frame and stops
// cleanly at the end state. Returns true while still running.
func stepAnimation() bool {
	a := &anim
	dur := animDurationMs()
	switch a.phase {
	case "rect":
		return stepRect(a, dur, func() {
			// A snapshot that arrived mid-transition skipped its
			// resize; apply it now that the window settled.
			if !collapsed {
				resizeForSnapshot()
			}
			// A standby edge detected mid-transition starts now.
			resumeStandby()
		})
	case "styleMorph":
		return stepRect(a, dur, resumeStandby)
	case "stdFadeOut":
		if time.Since(a.start).Milliseconds() >= stdFadeoutMs {
			a.phase = ""
			stopAnimTimer()
			if collapsed && inStandby {
				startStandbyFlash()
			}
			return false
		}
		procInvalidateRect.Call(hwndMain, 0, 0)
		return true
	case "stdFlash":
		visible, done := flashState(time.Since(a.start))
		a.visible = visible
		if done {
			a.phase = ""
			stopAnimTimer()
			if collapsed && inStandby {
				// Alarm done: ease into the standby strip.
				goStandbyStrip()
			}
			return false
		}
		procInvalidateRect.Call(hwndMain, 0, 0)
		return true
	case "stdShrink":
		return stepRect(a, dur, nil)
	case "stdRestore":
		return stepRect(a, dur, nil)
	}
	return false
}

// stepRect advances a rect-interpolating phase; onEnd runs after the
// final SetWindowPos lands.
func stepRect(a *hudAnim, durMs int32, onEnd func()) bool {
	t := float64(time.Since(a.start).Milliseconds()) / float64(durMs)
	if t >= 1 {
		procSetWindowPos.Call(hwndMain, ^uintptr(0),
			uintptr(a.toR.Left), uintptr(a.toR.Top),
			uintptr(a.toR.Right-a.toR.Left), uintptr(a.toR.Bottom-a.toR.Top), SWP_SHOWWINDOW)
		a.phase = ""
		stopAnimTimer()
		if onEnd != nil {
			onEnd()
		}
		procInvalidateRect.Call(hwndMain, 0, 0)
		return false
	}
	k := easeInOutCubic(t)
	procSetWindowPos.Call(hwndMain, ^uintptr(0),
		uintptr(lerpI(a.fromR.Left, a.toR.Left, k)), uintptr(lerpI(a.fromR.Top, a.toR.Top, k)),
		uintptr(lerpI(a.fromR.Right, a.toR.Right, k)-lerpI(a.fromR.Left, a.toR.Left, k)),
		uintptr(lerpI(a.fromR.Bottom, a.toR.Bottom, k)-lerpI(a.fromR.Top, a.toR.Top, k)),
		SWP_SHOWWINDOW)
	procInvalidateRect.Call(hwndMain, 0, 0)
	return true
}

func lerpI(a, b int32, k float64) int32 {
	return int32(float64(a) + k*float64(b-a))
}

func sameRect(a, b RECT) bool {
	return a.Left == b.Left && a.Top == b.Top && a.Right == b.Right && a.Bottom == b.Bottom
}

// standbyDimensions is the minimal standby strip: gauge-row sized when the
// style is gauges, taskbar strip otherwise. Pure - unit-tested.
func standbyDimensions(taskbarH int32) (int32, int32) {
	w := int32(240)
	if taskbarH >= 30 && taskbarH <= 100 {
		return w, taskbarH
	}
	return w, 40
}

// goStandbyStrip eases the window down to the standby strip, anchored at
// the current bottom-right corner.
func goStandbyStrip() {
	if hwndMain == 0 || !collapsed {
		return
	}
	w, h := standbyDimensions(taskbarHeight())
	fromR := windowRect(hwndMain)
	toR := RECT{Left: fromR.Right - w, Top: fromR.Bottom - h, Right: fromR.Right, Bottom: fromR.Bottom}
	startStandbyShrink(fromR, toR)
}
