package main

// Shared small helpers that several modules use. These were cut from
// main.go during the module split; they live here so window/paint code
// and the animation modules share one definition.

import (
	"time"
	"unsafe"
)

// windowRect returns the current outer window rectangle.
func windowRect(hwnd uintptr) RECT {
	var r RECT
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

// currentSnapshotValid reports whether the snapshot describes a
// signed-in account with bucket data (the only state standby applies to).
func currentSnapshotValid() bool {
	dataMu.RLock()
	defer dataMu.RUnlock()
	return currentSnapshot.SignedIn && len(currentSnapshot.Balances) > 0
}

// currentSnapshotLocked reports whether every bucket is exhausted,
// mirroring computeUnlock's locked condition.
func currentSnapshotLocked() bool {
	dataMu.RLock()
	s := cloneSnapshot(currentSnapshot)
	dataMu.RUnlock()
	if !s.SignedIn || len(s.Balances) == 0 {
		return false
	}
	_, locked := computeUnlock(s.Balances, timeNow())
	return locked
}

// timeNow is time.Now; a var so tests can pin the clock if ever needed.
var timeNow = time.Now
