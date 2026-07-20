package engine

import "testing"

func TestNegamaxMate(t *testing.T) {
	var nodes uint64

	pos := Position{}
	pos.PutPiece(H8, Black, King)
	pos.PutPiece(G7, Black, Pawn)
	pos.PutPiece(H7, Black, Pawn)
	pos.PutPiece(A1, White, King)
	pos.PutPiece(B1, White, Rook)
	pos.SideToMove = White

	pos.MakeMove(NewMove(B1, B8))

	score := Negamax(pos, 1, &nodes)
	t.Logf("negamax score for black (mated): %d", score)
	if score > -MateValue {
		t.Fatalf("want %d, get %d", -MateValue, score)
	}
}
