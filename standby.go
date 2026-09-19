package main

// Out-of-usage standby mode. When every bucket is exhausted the bar runs
// the alarm sequence (fade-out, 5x red flash, eased shrink) and then shows
// a countdown per exhausted bucket. Refill eases it back open. This
// replaces the old cooldown-strip behavior.
//
// State ownership: inStandby is the single alarm flag; anim runs the
// transitions. The expanded view is never resized by outage state.

import (
	"time"
)

// inStandby means the account is exhausted and the bar is in (or heading
// into) the standby strip. UI-thread owned.
var inStandby bool

// enterStandbyOrExpand runs on the UI thread after every snapshot resize:
// a newly exhausted account starts the fade-out -> flash -> shrink
// sequence; a refilled account eases the bar back open. No-op
// mid-animation so transitions never overlap.
func enterStandbyOrExpand() {
	if hwndMain == 0 || (previewMode && !previewLocked) || !currentSnapshotValid() {
		return
	}
	locked := currentSnapshotLocked()
	logDiagnostic("standby-check locked=%t inStandby=%t collapsed=%t anim=%q", locked, inStandby, collapsed, anim.phase)
	if locked && !inStandby {
		inStandby = true
		// Only the minimized bar runs the alarm; the expanded view is
		// NEVER resized by outage state. Mid-animation the start is
		// deferred: resumeStandby() fires when the running transition
		// completes.
		if collapsed && anim.phase == "" {
			startStandbyFadeOut()
		}
		return
	}
	if !locked && inStandby {
		inStandby = false
		if collapsed && anim.phase == "" {
			restoreBarSize()
		}
	}
}

// resumeStandby re-drives the standby chain after any animation finishes,
// so an edge detected mid-transition (e.g. locked snapshot arriving while
// the startup collapse is still easing) is never dropped. Safe to call
// always: it no-ops unless the bar is collapsed, idle, and flagged.
func resumeStandby() {
	if hwndMain == 0 || !collapsed || anim.phase != "" || !inStandby {
		return
	}
	if currentSnapshotLocked() {
		startStandbyFadeOut()
	} else {
		// Refilled while we waited: just re-open.
		inStandby = false
		restoreBarSize()
	}
}

// restoreBarSize eases the minimized bar from the standby strip back to
// its normal size once the buckets refill. The expanded view is never
// involved.
func restoreBarSize() {
	if hwndMain == 0 || !collapsed || anim.phase != "" {
		return
	}
	fromR := windowRect(hwndMain)
	x, y, w, h, _ := collapsedGeometryEx()
	startStandbyRestore(fromR, RECT{Left: x, Top: y, Right: x + w, Bottom: y + h})
}

// standbyBucketCountdowns returns one countdown per exhausted bucket with
// its qualifier label, ready for the standby strip. Pure - unit-tested.
func standbyBucketCountdowns(bs []Balance, now time.Time) []struct {
	Label  string
	Refill time.Time
} {
	type out = struct {
		Label  string
		Refill time.Time
	}
	var res []out
	for _, b := range bs {
		if balanceUsedPercent(b) < 100 {
			continue
		}
		label, _ := bucketQualifier(b)
		refill := earliestRefill([]Balance{b})
		if refill.IsZero() {
			continue
		}
		res = append(res, out{Label: label, Refill: refill})
	}
	return res
}

// standbyStripText renders the countdown line for the standby strip. With
// multiple exhausted buckets they join with " + ". Pure - unit-tested.
func standbyStripText(bs []Balance, now time.Time) string {
	items := standbyBucketCountdowns(bs, now)
	if len(items) == 0 {
		return "OUT OF USAGE"
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		parts = append(parts, it.Label+" in "+durationClock(time.Until(it.Refill)))
	}
	text := parts[0]
	if len(parts) > 1 {
		text = parts[0] + "  +  " + parts[1]
	}
	return text
}
