package main

import (
	"testing"
	"time"
)

func TestParsePreviewScript(t *testing.T) {
	cases := []struct {
		spec string
		want []time.Duration
	}{
		{"", nil},
		{"1500", []time.Duration{1500 * time.Millisecond}},
		{" 1200 , 6000 ", []time.Duration{1200 * time.Millisecond, 6000 * time.Millisecond}},
		{"0,-5,abc,,800", []time.Duration{800 * time.Millisecond}}, // junk skipped
	}
	for _, c := range cases {
		got := parsePreviewScript(c.spec)
		if len(got) != len(c.want) {
			t.Fatalf("parse(%q): %d events, want %d", c.spec, len(got), len(c.want))
		}
		for i, ev := range got {
			if ev.delay != c.want[i] {
				t.Fatalf("parse(%q)[%d] = %v, want %v", c.spec, i, ev.delay, c.want[i])
			}
		}
	}
}
