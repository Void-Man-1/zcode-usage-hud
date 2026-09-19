package main

// Standby painters: the alarm sequence frames (fade-out, red flash with
// fading "OUT OF USAGE", standby strip countdown) and the idle standby
// strip. All draw on the already-prepared target DC.

import (
	"time"
	"unsafe"
)

// standbyFlashColors are the two beats of the alarm: normal bar panel vs
// alarm red.
func standbyFlashColors(on bool) (bg, frame uint32) {
	if on {
		return c("standbyBg").v, c("bad").v
	}
	return c("windowBg").v, c("border").v
}

// paintStandby renders one frame of any standby sequence phase, plus the
// settled strip. `phase ""` means idle standby strip.
func paintStandby(hdc uintptr, rc RECT, s Snapshot, now time.Time) {
	phase := anim.phase

	// Idle strip (stdShrink/stdRestore mid-flight or after completion).
	// When the expanded view goes dormant the window keeps its full size,
	// so the alarm is drawn as a confined card instead of a full-screen
	// wash; only the minimized bar uses the whole panel.
	if phase == "" || phase == "stdShrink" || phase == "stdRestore" {
		bg := createBrush(c("windowBg").v)
		defer procDeleteObject.Call(bg)
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), bg)
		framePanel(hdc, rc, c("border").v)
		card := rc
		if rc.Bottom-rc.Top > 120 {
			card = RECT{Left: rc.Left + 16, Top: rc.Top + 12, Right: rc.Right - 16, Bottom: rc.Top + 58}
		}
		cardBg := createBrush(c("standbyBg").v)
		defer procDeleteObject.Call(cardBg)
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&card)), cardBg)
		framePanel(hdc, card, c("bad").v)
		text := standbyStripText(s.Balances, now)
		drawText(hdc, fontStatusCountdown, c("standbyText").v, text,
			card.Left+8, card.Top, card.Right-8, card.Bottom, DT_CENTER|DT_VCENTER|DT_SINGLELINE|DT_END_ELLIPSIS)
		return
	}

	if phase == "stdFadeOut" {
		// Content fading out: normal bar with a black veil rising over
		// the alarm-card area (the whole panel when minimized).
		paintCollapsed(hdc, rc, s, now)
		k := easeOutCubic(float64(time.Since(anim.start).Milliseconds()) / float64(stdFadeoutMs))
		card := rc
		if rc.Bottom-rc.Top > 120 {
			card = RECT{Left: rc.Left + 16, Top: rc.Top + 12, Right: rc.Right - 16, Bottom: rc.Top + 58}
		}
		veil := newAlphaSurface(card.Right-card.Left, card.Bottom-card.Top)
		if veil != nil {
			defer veil.release()
			veil.fill(c("standbyBg").v, k)
			veil.blend(hdc)
		}
		return
	}

	// stdFlash: background beats red while OUT OF USAGE fades in.
	// Expanded view: the flash is confined to the alarm card, the rest
	// of the panel stays untouched (bar-only contract).
	on := anim.visible
	bg, frame := standbyFlashColors(on)
	card := rc
	if rc.Bottom-rc.Top > 120 {
		card = RECT{Left: rc.Left + 16, Top: rc.Top + 12, Right: rc.Right - 16, Bottom: rc.Top + 58}
	}
	bgc := createBrush(bg)
	defer procDeleteObject.Call(bgc)
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&card)), bgc)
	framePanel(hdc, card, frame)

	// Text alpha ramps over the first beat, then holds.
	elapsed := time.Since(anim.start).Milliseconds()
	alpha := clampF(float64(elapsed)/float64(stdTextFadeMs), 0, 1)
	cw, ch := card.Right-card.Left, card.Bottom-card.Top
	surf := newAlphaSurface(cw, ch)
	if surf != nil {
		defer surf.release()
		surf.text(fontStatusCountdown, c("textBright").v, "OUT OF USAGE",
			RECT{0, 0, cw, ch}, DT_CENTER|DT_VCENTER|DT_SINGLELINE, alpha)
		surf.blend(hdc)
	}
}
