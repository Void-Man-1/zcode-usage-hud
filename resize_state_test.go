package main

import (
	"testing"
	"time"
)

// Drives resizeForSnapshot's three states against the real shared state:
// 1. applying records winHeight == the snapshot's height (no desync),
// 2. animating skips WITHOUT recording (the pre-fix bug recorded anyway),
// 3. clearing the animation lets a re-run apply the pending height.
func TestResizeForSnapshotSkipsWithoutRecording(t *testing.T) {
	oldH := winHeight
	oldColl := collapsed
	oldAnim := anim
	oldSnap := func() Snapshot {
		dataMu.RLock()
		defer dataMu.RUnlock()
		return cloneSnapshot(currentSnapshot)
	}()
	defer func() {
		winHeight = oldH
		collapsed = oldColl
		anim = oldAnim
		dataMu.Lock()
		currentSnapshot = oldSnap
		dataMu.Unlock()
	}()

	collapsed = false
	anim = hudAnim{}
	winHeight = 900
	dataMu.Lock()
	currentSnapshot = previewSnapshot()
	dataMu.Unlock()

	want := expandedClientHeight(currentSnapshot)
	if want == 900 {
		t.Fatalf("fixture degenerate: snapshot height %d equals baseline", want)
	}

	// Animating: skip, and winHeight must NOT advance (the fix).
	anim = hudAnim{phase: "rect", start: time.Now()}
	resizeForSnapshot()
	if winHeight != 900 {
		t.Fatalf("animating: winHeight recorded %d, want untouched 900", winHeight)
	}

	// Animation cleared: the pending resize now applies.
	anim = hudAnim{}
	resizeForSnapshot()
	if winHeight != want {
		t.Fatalf("after anim: winHeight %d, want %d (pending resize lost?)", winHeight, want)
	}

	// Collapsed: never touches the expanded height.
	collapsed = true
	resizeForSnapshot()
	if winHeight != want {
		t.Fatalf("collapsed: winHeight changed to %d, want %d", winHeight, want)
	}
}
