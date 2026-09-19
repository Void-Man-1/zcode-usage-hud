package main

// Speedometer gauge style: one GDI arc gauge per bucket, all gauges on a
// single horizontal line. Geometry helpers are pure and unit-tested; the
// paint entry takes alpha for the animated bars<->gauges crossfade.

import (
	"fmt"
	"math"
	"syscall"
)

var (
	procCreatePen = syscall.NewLazyDLL("gdi32.dll").NewProc("CreatePen")
	procMoveToEx  = syscall.NewLazyDLL("gdi32.dll").NewProc("MoveToEx")
	procLineTo    = syscall.NewLazyDLL("gdi32.dll").NewProc("LineTo")
	procAngleArc  = syscall.NewLazyDLL("gdi32.dll").NewProc("AngleArc")
	procEllipse   = syscall.NewLazyDLL("gdi32.dll").NewProc("Ellipse")
	procGetDC     = user32.NewProc("GetDC")
	procReleaseDC = user32.NewProc("ReleaseDC")
)

const psSolid = 0

// gaugeLayout computes one row of n equal gauge cells inside
// [left,right]x[top,bottom]. Pure - unit-tested.
func gaugeLayout(n int, left, top, right, bottom int32) []RECT {
	if n <= 0 {
		return nil
	}
	if n > 6 {
		n = 6
	}
	gap := int32(8)
	total := right - left
	cell := (total - gap*int32(n-1)) / int32(n)
	if cell < 40 {
		cell = 40
	}
	out := make([]RECT, 0, n)
	x := left
	for i := 0; i < n; i++ {
		out = append(out, RECT{Left: x, Top: top, Right: x + cell, Bottom: bottom})
		x += cell + gap
	}
	return out
}

// gaugeRowHeight returns the collapsed-bar height needed for one gauge
// row (plus the slim button strip). Pure - unit-tested.
func gaugeRowHeight() int32 { return 74 }

// drawGauge paints one speedometer gauge in cell rc at `alpha` (0..1,
// used by the style-morph crossfade). The arc sweeps 240° from 150° to
// 30° (clockwise through 270°), needle at the remaining fraction.
func drawGauge(hdc uintptr, rc RECT, remaining float64, label string, alpha float64) {
	if alpha <= 0.02 {
		return
	}
	w := rc.Right - rc.Left
	h := rc.Bottom - rc.Top
	cx := float64(rc.Left) + float64(w)/2
	cy := float64(rc.Top) + float64(h)*0.58
	radius := math.Min(float64(w), float64(h)*1.5) * 0.40

	// Colors blend toward the panel color as alpha drops, so the
	// crossfade reads as a real fade rather than a hard pop.
	track := c("barTrack").blend(c("windowBg"), 1-alpha)
	arc := c("gaugeArc").blend(c("windowBg"), 1-alpha)
	needle := c("gaugeNeedle").blend(c("windowBg"), 1-alpha)
	text := c("textBright").blend(c("windowBg"), 1-alpha)
	muted := c("textMuted").blend(c("windowBg"), 1-alpha)

	// Track arc (full sweep) then value arc.
	strokeArc(hdc, cx, cy, radius, 150, 390, track.v, 5)
	// Sweep proportional to remaining, drawn from the left end.
	sweep := 240 * clampF(remaining/100, 0, 1)
	if sweep > 0.5 {
		strokeArc(hdc, cx, cy, radius, 150, 150+sweep, arc.v, 5)
	}

	// Needle.
	ang := (150 + sweep) * math.Pi / 180
	nx := cx + math.Cos(ang)*radius*0.78
	ny := cy + math.Sin(ang)*radius*0.78
	drawLine(hdc, cx, cy, nx, ny, needle.v, 2)

	// Hub dot.
	hubR := radius * 0.07
	if hubR < 2 {
		hubR = 2
	}
	fillCircle(hdc, cx, cy, hubR, needle.v)

	// % text inside the gauge, label under it.
	pct := fmt.Sprintf("%.0f%%", remaining)
	drawText(hdc, fontBody, text.v, pct,
		rc.Left, int32(cy)-int32(radius*0.30), rc.Right, int32(cy)+int32(radius*0.30),
		DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	drawText(hdc, fontSmall, muted.v, label,
		rc.Left, rc.Bottom-int32(float64(h)*0.22), rc.Right, rc.Bottom-2,
		DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}

// drawGaugeRow paints the whole horizontal gauge row for the given
// buckets (pending grants keep their activation countdown as the label).
func drawGaugeRow(hdc uintptr, bs []Balance, pend []PendingGrant, left, top, right, bottom int32, alpha float64) {
	cells := gaugeLayout(len(bs)+len(pend), left, top, right, bottom)
	if cells == nil {
		return
	}
	i := 0
	for _, b := range bs {
		if i >= len(cells) {
			break
		}
		label, _ := bucketQualifier(b)
		drawGauge(hdc, cells[i], remainingPct(b), label, alpha)
		i++
	}
	for range pend {
		if i >= len(cells) {
			break
		}
		drawGauge(hdc, cells[i], 0, "PROMO SOON", alpha)
		i++
	}
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- GDI primitives (each self-contained: select, draw, restore) ---

// strokeArc draws a circular arc from startDeg to endDeg (screen angles:
// 0 = right, increasing counterclockwise on screen, 270 = top).
func strokeArc(hdc uintptr, cx, cy, radius float64, startDeg, endDeg float64, color uint32, width int32) {
	pen, _, _ := procCreatePen.Call(psSolid, uintptr(width), uintptr(color))
	if pen == 0 {
		return
	}
	defer procDeleteObject.Call(pen)
	oldp, _, _ := procSelectObject.Call(hdc, pen)
	defer procSelectObject.Call(hdc, oldp)
	sx := uintptr(cx + radius*math.Cos(startDeg*math.Pi/180))
	sy := uintptr(cy + radius*math.Sin(startDeg*math.Pi/180))
	procMoveToEx.Call(hdc, sx, sy, 0)
	procAngleArc.Call(hdc, uintptr(int64(cx)), uintptr(int64(cy)), uintptr(radius),
		uintptr(int32(startDeg)), uintptr(int32(endDeg-startDeg)))
}

// drawLine draws a solid segment between two points.
func drawLine(hdc uintptr, x1, y1, x2, y2 float64, color uint32, width int32) {
	pen, _, _ := procCreatePen.Call(psSolid, uintptr(width), uintptr(color))
	if pen == 0 {
		return
	}
	defer procDeleteObject.Call(pen)
	oldp, _, _ := procSelectObject.Call(hdc, pen)
	defer procSelectObject.Call(hdc, oldp)
	procMoveToEx.Call(hdc, uintptr(int64(x1)), uintptr(int64(y1)), 0)
	procLineTo.Call(hdc, uintptr(int64(x2)), uintptr(int64(y2)))
}

// fillCircle paints a filled disc with the given color.
func fillCircle(hdc uintptr, cx, cy, r float64, color uint32) {
	brush, _, _ := procCreateSolidBrush.Call(uintptr(color))
	if brush == 0 {
		return
	}
	defer procDeleteObject.Call(brush)
	oldb, _, _ := procSelectObject.Call(hdc, brush)
	defer procSelectObject.Call(hdc, oldb)
	oldp, _, _ := procSelectObject.Call(hdc, getStockNullPen())
	defer procSelectObject.Call(hdc, oldp)
	procEllipse.Call(hdc,
		uintptr(int64(cx-r)), uintptr(int64(cy-r)), uintptr(int64(cx+r)), uintptr(int64(cy+r)))
}

var procGetStockObject = syscall.NewLazyDLL("gdi32.dll").NewProc("GetStockObject")

// getStockNullPen returns the NULL_PEN stock object (index 8) so filled
// circles draw without an outline.
func getStockNullPen() uintptr {
	r, _, _ := procGetStockObject.Call(nullPenIdx)
	return r
}

const nullPenIdx = 8 // NULL_PEN
