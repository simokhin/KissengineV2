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

	score := s.Negamax(&pos, 1, 1, alpha, beta, history, false, 0)
	t.Logf("negamax score for black (mated): %d", score)
	if score > -MateValue {
		t.Fatalf("want %d, get %d", -MateValue, score)
	}
}

// TestPlyBeyondKillersBounds guards against a regression where accessing
// s.killers[ply] without a bounds check panics once ply reaches len(s.killers)
// (64) -- reachable in real games via check extensions, which grow ply
// without consuming depth.
func TestPlyBeyondKillersBounds(t *testing.T) {
	s := &SearchState{ctx: context.Background()}
	pos := StartPos()

	deepPly := len(s.killers)

	if !s.canReduce(*pos, NewMove(B1, C3), 5, 5, deepPly) {
		t.Errorf("canReduce at ply=%d: want true, got false", deepPly)
	}

	s.storeKiller(deepPly, NewMove(B1, C3))

	score := s.orderScore(*pos, deepPly, NewMove(B1, C3), Move(0))
	t.Logf("orderScore at ply=%d: %d", deepPly, score)
}
