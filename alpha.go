package main

// Alpha compositing for fades. GDI has no per-draw alpha, so fading
// layers (standby overlay, gauge crossfade) render into a 32bpp DIB
// section with premultiplied alpha and AlphaBlend onto the target.
// Surfaces are small (bar-sized) and short-lived; cost is negligible.

import (
	"syscall"
	"unsafe"
)

var (
	procCreateDIBSection = gdi32.NewProc("CreateDIBSection")
	// AlphaBlend is exported by msimg32.dll, not gdi32 — resolving it
	// against gdi32 panics the LazyProc on first paint.
	procAlphaBlend = syscall.NewLazyDLL("msimg32.dll").NewProc("AlphaBlend")
)

const (
	BI_RGB         = 0
	DIB_RGB_MODE   = 0 // DIB_RGB_COLORS
	AC_SRC_OVER    = 0
	AC_SRC_ALPHA   = 1
	cbSrccopyBlend = 0x00CC0020
)

type bitmapInfo struct {
	Header struct {
		Size          uint32
		Width         int32
		Height        int32
		Planes        uint16
		BitCount      uint16
		Compression   uint32
		SizeImage     uint32
		XPelsPerMeter int32
		YPelsPerMeter int32
		ClrUsed       uint32
		ClrImportant  uint32
	}
}

type alphaSurface struct {
	dc   uintptr
	bmp  uintptr
	old  uintptr
	bits []byte
	w, h int32
}

// newAlphaSurface creates a top-down 32bpp ARGB surface.
func newAlphaSurface(w, h int32) *alphaSurface {
	if w <= 0 || h <= 0 {
		return nil
	}
	dc, _, _ := procCreateCompatibleDC.Call(targetScreenDC())
	if dc == 0 {
		return nil
	}
	var bi bitmapInfo
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.Width = w
	bi.Header.Height = -h // top-down
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = BI_RGB
	var bitsPtr unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), DIB_RGB_MODE, uintptr(unsafe.Pointer(&bitsPtr)), 0, 0)
	if bmp == 0 || bitsPtr == nil {
		procDeleteDC.Call(dc)
		return nil
	}
	old, _, _ := procSelectObject.Call(dc, bmp)
	n := int(w) * int(h) * 4
	// bitsPtr is unsafe.Pointer to DIB memory owned by the section for
	// the surface's lifetime, so slicing it directly is sound.
	return &alphaSurface{
		dc: dc, bmp: bmp, old: old,
		bits: unsafe.Slice((*byte)(bitsPtr), n),
		w:    w, h: h,
	}
}

func (s *alphaSurface) release() {
	if s == nil {
		return
	}
	procSelectObject.Call(s.dc, s.old)
	procDeleteObject.Call(s.bmp)
	procDeleteDC.Call(s.dc)
}

// fill paints the whole surface premultiplied.
func (s *alphaSurface) fill(color uint32, alpha float64) {
	s.fillRect(RECT{0, 0, s.w, s.h}, color, alpha)
}

// fillRect paints one rect premultiplied. color is 0x00BBGGRR.
func (s *alphaSurface) fillRect(rc RECT, color uint32, alpha float64) {
	if s == nil || alpha <= 0 {
		return
	}
	a := clampF(alpha, 0, 1)
	r := byte(float64(color&0xFF) * a)
	g := byte(float64((color>>8)&0xFF) * a)
	b := byte(float64((color>>16)&0xFF) * a)
	aa := byte(a * 255)
	left := clampI32(rc.Left, 0, s.w)
	top := clampI32(rc.Top, 0, s.h)
	right := clampI32(rc.Right, 0, s.w)
	bottom := clampI32(rc.Bottom, 0, s.h)
	for y := top; y < bottom; y++ {
		row := s.bits[int(y)*int(s.w)*4:]
		for x := left; x < right; x++ {
			i := int(x) * 4
			row[i] = r
			row[i+1] = g
			row[i+2] = b
			row[i+3] = aa
		}
	}
}

// text draws antialiased text onto the surface: the glyph coverage comes
// from a white-on-black mask render, mapped into per-pixel alpha.
func (s *alphaSurface) text(font uintptr, color uint32, text string, rc RECT, flags uint32, alpha float64) {
	if s == nil || alpha <= 0 || text == "" {
		return
	}
	left := clampI32(rc.Left, 0, s.w)
	top := clampI32(rc.Top, 0, s.h)
	right := clampI32(rc.Right, 0, s.w)
	bottom := clampI32(rc.Bottom, 0, s.h)
	w, h := right-left, bottom-top
	if w <= 0 || h <= 0 {
		return
	}
	// Mask: 32bpp DIB, black bg, white text. Coverage = pixel value.
	mask := newAlphaSurface(w, h)
	if mask == nil {
		return
	}
	defer mask.release()
	mask.fillRect(RECT{0, 0, w, h}, 0x00000000, 1)
	procSetBkMode.Call(mask.dc, TRANSPARENT)
	drawText(mask.dc, font, 0xFFFFFF, text, 0, 0, w, h, flags)

	tr := byte(float64(color&0xFF) * alpha)
	tg := byte(float64((color>>8)&0xFF) * alpha)
	tb := byte(float64((color>>16)&0xFF) * alpha)
	for y := 0; y < int(h); y++ {
		mrow := mask.bits[y*int(w)*4:]
		drow := s.bits[(int(top)+y)*int(s.w)*4:]
		for x := 0; x < int(w); x++ {
			cov := mrow[x*4] // red channel of the grayscale coverage
			if cov == 0 {
				continue
			}
			di := (int(left)+x)*4 + 3 // alpha slot of the destination pixel
			// coverage scales the text alpha; keep the strongest source
			ca := byte(float64(cov) / 255.0 * alpha * 255.0)
			if ca > drow[0] && drow[0] == 0 {
				// empty destination pixel: write text color
				di -= 3
				drow[di] = tr
				drow[di+1] = tg
				drow[di+2] = tb
				drow[di+3] = ca
			} else if ca > 0 {
				// over a filled pixel: only raise alpha toward text
				// (text wins visually because it is brighter)
				if ca > drow[3] {
					di -= 3
					drow[di] = tr
					drow[di+1] = tg
					drow[di+2] = tb
					drow[di+3] = ca
				}
			}
		}
	}
}

// blend composites the surface onto dst at (0,0).
func (s *alphaSurface) blend(dstDC uintptr) {
	if s == nil {
		return
	}
	bf := uintptr(AC_SRC_OVER | AC_SRC_ALPHA<<8 | 255<<16)
	procAlphaBlend.Call(dstDC, 0, 0, uintptr(s.w), uintptr(s.h),
		s.dc, 0, 0, uintptr(s.w), uintptr(s.h), bf)
}

// targetScreenDC returns a scratch screen DC for offscreen surfaces.
func targetScreenDC() uintptr {
	hwnd := hwndMain
	if hwnd == 0 {
		return 0
	}
	// A window's DC class works for compatible-bitmap creation; the
	// main window is always present by the time surfaces are used.
	dc, _, _ := procGetDC.Call(hwnd)
	// Leaked intentionally per call is wasteful: use a cached DC.
	if screenDCCache != 0 {
		procReleaseDC.Call(hwnd, dc)
		return screenDCCache
	}
	screenDCCache = dc
	return dc
}

var screenDCCache uintptr
