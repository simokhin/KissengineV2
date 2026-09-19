package engine

import "testing"

func TestGenerateMoves(t *testing.T) {
	pos := StartPos()

	var list MoveList
	GenerateMoves(*pos, White, &list)
	if list.count != 20 {
		t.Fatalf("want 20, get %d", list.count)
	}
}

func TestGenerateRookMoves(t *testing.T) {
	pos := StartPos()

	var list MoveList
	GenerateRookMoves(*pos, White, &list)
	if list.count != 0 {
		t.Fatalf("want 0, get %d", list.count)
	}
}

func TestGenerateQueenMoves(t *testing.T) {
	pos := StartPos()

	var list MoveList
	GenerateQueenMoves(*pos, White, &list)
	if list.count != 0 {
		t.Fatalf("want 0, get %d", list.count)
	}
}

func TestGenerateBishopMoves(t *testing.T) {
	pos := StartPos()

	var list MoveList
	GenerateBishopMoves(*pos, White, &list)
	if list.count != 0 {
		t.Fatalf("want 0, get %d", list.count)
	}
}

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

func TestGeneratePawnMoves(t *testing.T) {
	pos := StartPos()

	var list MoveList
	GeneratePawnMoves(*pos, White, &list)
	if list.count != 16 {
		t.Fatalf("want 16, get %d", list.count)
	}

	list.Reset()
	GeneratePawnMoves(*pos, Black, &list)
	if list.count != 16 {
		t.Fatalf("want 16, get %d", list.count)
	}
}

func TestGeneratePawnCaptures(t *testing.T) {
	pos := Position{}

	pos.PutPiece(A5, Black, Pawn)
	pos.PutPiece(B4, White, Pawn)
	pos.PutPiece(A4, White, Pawn)

	var list MoveList
	GeneratePawnMoves(pos, Black, &list)
	if list.count != 1 {
		t.Fatalf("want 1, get %d", list.count)
	}
}

func TestGenerateKingMoves(t *testing.T) {
	pos := StartPos()

	var list MoveList
	GenerateKingMoves(*pos, White, &list)
	if list.count > 0 {
		t.Fatalf("want 0, get %d", list.count)
	}
}

func TestGenerateKnightMoves(t *testing.T) {
	pos := StartPos()

	var list MoveList
	GenerateKnightMoves(*pos, White, &list)
	if list.count != 4 {
		t.Fatalf("want 4 moves; get %d", list.count)
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
