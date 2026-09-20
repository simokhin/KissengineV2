package engine

import (
	"testing"
	"unsafe"
)

// TestTTSlotSize pins the packed layout: four slots per cache line depends on it.
func TestTTSlotSize(t *testing.T) {
	if got := unsafe.Sizeof(ttSlot{}); got != ttSlotBytes {
		t.Fatalf("ttSlot is %d bytes, want %d", got, ttSlotBytes)
	}
}

func TestTTRoundTrip(t *testing.T) {
	ClearHash()
	defer ClearHash()

	tests := []struct {
		name  string
		depth int
		score int
		flag  TTFlag
		move  Move
	}{
		{"positive", 7, 123, Exact, Move(0x1234)},
		{"negative", 3, -456, LowerBound, Move(1)},
		{"mate win", 12, MateValue - 5 + 20, UpperBound, Move(0xFFF)},
		{"mate loss", 12, -(MateValue - 5 + 20), Exact, Move(0)},
		{"depth clamped", 200, 0, Exact, Move(2)},
	}

	for i, tt := range tests {
		hash := uint64(0x9E3779B97F4A7C15) * uint64(i+1)
		ttStore(hash, tt.depth, tt.score, tt.flag, tt.move)

		entry, found := ttProbe(hash)
		if !found {
			t.Fatalf("%s: stored entry not found", tt.name)
		}

		wantDepth := min(tt.depth, 127)
		if entry.depth != wantDepth || entry.score != tt.score || entry.flag != tt.flag || entry.bestMove != tt.move || entry.hash != hash {
			t.Errorf("%s: got %+v, want depth %d score %d flag %d move %d",
				tt.name, entry, wantDepth, tt.score, tt.flag, tt.move)
		}
	}
}

// TestTTReplacement: a different position always replaces the slot's owner,
// the same position only when the new depth is at least as large.
func TestTTReplacement(t *testing.T) {
	ClearHash()
	defer ClearHash()

	h := uint64(0xABCDEF)
	collider := h + uint64(len(tTable)) // same index, different key

	ttStore(h, 8, 10, Exact, Move(1))
	ttStore(h, 5, 20, Exact, Move(2)) // shallower, same position: kept out
	if e, _ := ttProbe(h); e.score != 10 {
		t.Errorf("shallower store replaced the deeper entry: %+v", e)
	}

	ttStore(h, 8, 30, Exact, Move(3)) // equal depth: replaces
	if e, _ := ttProbe(h); e.score != 30 {
		t.Errorf("equal-depth store did not replace: %+v", e)
	}

	ttStore(collider, 1, 40, Exact, Move(4)) // other position, any depth: replaces
	if _, found := ttProbe(h); found {
		t.Error("probe of the evicted position still reports found")
	}
	if e, found := ttProbe(collider); !found || e.score != 40 {
		t.Errorf("colliding position not stored: %+v found=%v", e, found)
	}
}

func TestSetHashSizeAndClear(t *testing.T) {
	defer SetHashSize(DefaultHashMB)

	SetHashSize(1)
	if len(tTable) != 1<<16 { // 1MB / 16 bytes
		t.Fatalf("1MB table has %d slots, want %d", len(tTable), 1<<16)
	}

	// Sizes that aren't a power of two round down.
	SetHashSize(3)
	if len(tTable) != 1<<17 {
		t.Fatalf("3MB table has %d slots, want %d", len(tTable), 1<<17)
	}
	if ttMask != uint64(len(tTable))-1 {
		t.Fatalf("mask %x doesn't match length %d", ttMask, len(tTable))
	}

	// Below the minimum, clamp to 1MB.
	SetHashSize(0)
	if len(tTable) != 1<<16 {
		t.Fatalf("0MB request gave %d slots, want %d", len(tTable), 1<<16)
	}

	ttStore(42, 4, 1, Exact, Move(9))
	ClearHash()
	if _, found := ttProbe(42); found {
		t.Error("entry survived ClearHash")
	}
}
