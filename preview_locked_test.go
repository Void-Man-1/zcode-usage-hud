package main

import (
	"testing"
	"time"
)

func TestPreviewSnapshotLocked(t *testing.T) {
	previewMode = true
	defer func() { previewMode = false }()

	// Locked phase: all buckets exhausted, recurring period, unlock in the future.
	previewLocked = true
	previewLaunch = time.Now()
	s := previewSnapshot()
	if len(s.Balances) != 2 {
		t.Fatalf("locked balances = %d, want 2 (same buckets as normal preview)", len(s.Balances))
	}
	for _, b := range s.Balances {
		if b.Remaining != 0 || b.Total <= 0 {
			t.Fatalf("locked bucket not exhausted: total=%d remaining=%d", b.Total, b.Remaining)
		}
		if !isRecurringPeriod(b.Period) {
			t.Fatalf("locked bucket period %q must be recurring for computeUnlock", b.Period)
		}
	}
	if _, locked := computeUnlock(s.Balances, time.Now()); !locked {
		t.Fatal("computeUnlock must report locked for the locked fixture")
	}

	// After the 8s fixture window: normal preview balances return.
	previewLaunch = time.Now().Add(-9 * time.Second)
	s2 := previewSnapshot()
	if len(s2.Balances) != 2 {
		t.Fatalf("refilled balances = %d, want 2 (normal preview)", len(s2.Balances))
	}
	if _, locked := computeUnlock(s2.Balances, time.Now()); locked {
		t.Fatal("refilled fixture must not be locked")
	}

	previewLocked = false
}
