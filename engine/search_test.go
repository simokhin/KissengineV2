package engine

import (
	"context"
	"testing"
)

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

	alpha := Minimum
	beta := Maximum
	history := []uint64{pos.Hash()}

	score := Negamax(context.Background(), pos, 1, &nodes, alpha, beta, history, false)
	t.Logf("negamax score for black (mated): %d", score)
	if score > -MateValue {
		t.Fatalf("want %d, get %d", -MateValue, score)
	}
}
