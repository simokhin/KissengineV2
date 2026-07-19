package engine

import "testing"

func TestPawnCaptures(t *testing.T) {
	pos := Position{}

	pos.PutPiece(A5, Black, Pawn)
	pos.PutPiece(B4, White, Pawn)

	pawnCaptures := PawnCaptures(pos, Black)
	if pawnCaptures.PopCount() != 1 {
		t.Fatalf("want 1; get %d", pawnCaptures.PopCount())
	}
}

func TestDoublePawnPush(t *testing.T) {
	pos := StartPos()

	targetRank := DoublePawnPush(*pos, White)
	if targetRank.PopCount() != 8 {
		t.Fatalf("want 8, get %d", targetRank.PopCount())
	}

	targetRank = DoublePawnPush(*pos, Black)
	if targetRank.PopCount() != 8 {
		t.Fatalf("want 8, get %d", targetRank.PopCount())
	}
}

func TestPawnPush(t *testing.T) {
	pos := StartPos()

	targetRank := PawnPush(*pos, White)
	if targetRank.PopCount() != 8 {
		t.Fatalf("want 8, get %d", targetRank.PopCount())
	}

	targetRank = PawnPush(*pos, Black)
	if targetRank.PopCount() != 8 {
		t.Fatalf("want 8, get %d", targetRank.PopCount())
	}
}

func TestGenerateKingMoves(t *testing.T) {
	pos := StartPos()

	moves := GenerateKingMoves(*pos, White)
	if len(moves) > 0 {
		t.Fatalf("want 0, get %d", len(moves))
	}
}

func TestGenerateKnightMoves(t *testing.T) {
	pos := StartPos()

	moves := GenerateKnightMoves(*pos, White)
	if len(moves) != 4 {
		t.Fatalf("want 4 moves; get %d", len(moves))
	}
}

func TestKingAttacks(t *testing.T) {
	kingAttacksCount := KingAttacksFrom(E4).PopCount()
	if kingAttacksCount != 8 {
		t.Fatalf("want 8; get %d", kingAttacksCount)
	}

	kingAttacksCount = KingAttacksFrom(A1).PopCount()
	if kingAttacksCount != 3 {
		t.Fatalf("want 3; get %d", kingAttacksCount)
	}
}

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
