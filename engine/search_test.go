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
	if score > -(MateValue - 1000) {
		t.Fatalf("want score <= %d, got %d", -(MateValue - 1000), score)
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

// TestMateScorePlyAdjustment guards against a regression in the TT mate-score
// ply adjustment: storing a mate score found deep in the tree and retrieving
// it via transposition at a shallower ply must report the mate as closer
// (fewer plies to mate), not repeat the original, now-stale distance.
func TestMateScorePlyAdjustment(t *testing.T) {
	winScore := MateValue - 5 // mate found at ply 5 from root
	loseScore := -(MateValue - 5)

	// Round-trip at the same ply must be lossless.
	if got := adjustMateForRetreve(adjustMateForStore(winScore, 3), 3); got != winScore {
		t.Errorf("round-trip win at ply 3: want %d, got %d", winScore, got)
	}
	if got := adjustMateForRetreve(adjustMateForStore(loseScore, 3), 3); got != loseScore {
		t.Errorf("round-trip lose at ply 3: want %d, got %d", loseScore, got)
	}

	// Stored at ply 3, retrieved at ply 1 (shallower transposition, 2 plies
	// closer to root) -- the mate should now look 2 plies closer too.
	stored := adjustMateForStore(winScore, 3)
	if got := adjustMateForRetreve(stored, 1); got != winScore+2 {
		t.Errorf("win stored@3 retrieved@1: want %d, got %d", winScore+2, got)
	}

	stored = adjustMateForStore(loseScore, 3)
	if got := adjustMateForRetreve(stored, 1); got != loseScore-2 {
		t.Errorf("lose stored@3 retrieved@1: want %d, got %d", loseScore-2, got)
	}

	// Non-mate scores must pass through unchanged.
	if got := adjustMateForStore(150, 5); got != 150 {
		t.Errorf("non-mate store: want 150, got %d", got)
	}
	if got := adjustMateForRetreve(150, 5); got != 150 {
		t.Errorf("non-mate retrieve: want 150, got %d", got)
	}
}
