package engine

import "testing"

func TestKnightAttacks(t *testing.T) {
	knightAttacksCount := KnightAttacksFrom(D4).PopCount()
	if knightAttacksCount != 8 {
		t.Fatalf("want 8; get %d", knightAttacksCount)
	}

	knightAttacksCount = KnightAttacksFrom(A1).PopCount()
	if knightAttacksCount != 2 {
		t.Fatalf("want 2; get %d", knightAttacksCount)
	}
}
