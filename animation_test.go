package main

import (
	"math"
	"testing"
	"time"
)

func TestFlashState(t *testing.T) {
	// First blink on.
	if visible, done := flashState(0); !visible || done {
		t.Fatalf("t=0: visible=%t done=%t, want true false", visible, done)
	}
	// Off during the second half of cycle 1.
	if visible, done := flashState(600 * time.Millisecond); visible || done {
		t.Fatalf("t=600ms: visible=%t done=%t, want false false", visible, done)
	}
	// On at the start of cycle 5.
	if visible, done := flashState(4400 * time.Millisecond); !visible || done {
		t.Fatalf("t=4400ms: visible=%t done=%t, want true false", visible, done)
	}
	// All five blinks complete at 5.5 s.
	if visible, done := flashState(5500 * time.Millisecond); visible || !done {
		t.Fatalf("t=5500ms: visible=%t done=%t, want false true", visible, done)
	}
	// Well past the end stays done.
	if _, done := flashState(10 * time.Second); !done {
		t.Fatal("t=10s: done=false, want true")
	}
}

func TestEaseInOutCubic(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{-1, 0}, {0, 0}, {0.25, 0.0625}, {0.5, 0.5}, {0.75, 0.9375}, {1, 1}, {2, 1},
	}
	for _, c := range cases {
		if got := easeInOutCubic(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("easeInOutCubic(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	// Monotonic non-decreasing across the sweep.
	prev := 0.0
	for i := 0; i <= 100; i++ {
		v := easeInOutCubic(float64(i) / 100)
		if v < prev {
			t.Fatalf("non-monotonic at %d: %v < %v", i, v, prev)
		}
		prev = v
	}
}

func TestCooldownDimensions(t *testing.T) {
	// Normal taskbar heights: bar matches the taskbar.
	for _, th := range []int32{30, 40, 48, 100} {
		w, h := cooldownDimensions(th)
		if w != 240 || h != th {
			t.Errorf("cooldownDimensions(%d) = %d,%d, want 240,%d", th, w, h, th)
		}
	}
	// Out-of-range heights fall back to 40.
	for _, th := range []int32{0, 29, 101, 500} {
		w, h := cooldownDimensions(th)
		if w != 240 || h != 40 {
			t.Errorf("cooldownDimensions(%d) = %d,%d, want 240,40", th, w, h)
		}
	}
}

func TestAccentColorFallback(t *testing.T) {
	accentSet, accentValue = false, 0xFF0000
	if got := accentColor(0x00FF00); got != 0x00FF00 {
		t.Fatalf("no accent set: got %x, want original 00ff00", got)
	}
	accentSet, accentValue = true, 0xFF0000
	if got := accentColor(0x00FF00); got != 0xFF0000 {
		t.Fatalf("accent set: got %x, want ff0000", got)
	}
}
