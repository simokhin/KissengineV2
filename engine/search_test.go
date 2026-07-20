package engine

import (
	"context"
	"testing"
)

func TestNegamaxMate(t *testing.T) {
	s := &SearchState{ctx: context.Background()}

	pos := Position{}
	pos.PutPiece(H8, Black, King)
	pos.PutPiece(G7, Black, Pawn)
	pos.PutPiece(H7, Black, Pawn)
	pos.PutPiece(A1, White, King)
	pos.PutPiece(B1, White, Rook)
	pos.SideToMove = White

	history := []uint64{pos.Hash()}
	pos.MakeMove(NewMove(B1, B8))
	history = append(history, pos.Hash())

	alpha := Minimum
	beta := Maximum

	score := s.Negamax(&pos, 1, 1, alpha, beta, history, false)
	t.Logf("negamax score for black (mated): %d", score)
	if score > -MateValue {
		t.Fatalf("want %d, get %d", -MateValue, score)
	}
}
