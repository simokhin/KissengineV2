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

func TestGenerateLegalNoisyMoves(t *testing.T) {
	// Only the queen promotion is noisy: the king's quiet moves and the three
	// underpromotions must be left out.
	pos := ParseFEN("4k3/P7/8/8/8/8/8/4K3 w - - 0 1")

	var list MoveList
	GenerateLegalNoisyMoves(*pos, White, &list)
	if list.count != 1 || list.Slice()[0] != NewPromotionMove(A7, A8, Queen) {
		t.Fatalf("want exactly a7a8q, got %d moves", list.count)
	}

	// Captures stay in, including a capture with underpromotion.
	pos = ParseFEN("1r2k3/P7/8/8/8/8/8/4K3 w - - 0 1")
	GenerateLegalNoisyMoves(*pos, White, &list)
	got := map[string]bool{}
	for _, m := range list.Slice() {
		got[m.UCI()] = true
	}
	for _, want := range []string{"a7a8q", "a7b8q", "a7b8r", "a7b8b", "a7b8n"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	if got["a7a8n"] || got["a7a8r"] || got["a7a8b"] {
		t.Errorf("quiet underpromotions must not be generated: %v", got)
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
