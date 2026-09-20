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

// TestQuiescenceSeesQuietPromotion guards against quiescence only looking at
// captures: a pawn on the 7th with nothing to capture must still be seen
// promoting, otherwise a search stopping right before the promotion misjudges
// the position by about a queen.
func TestQuiescenceSeesQuietPromotion(t *testing.T) {
	pos := ParseFEN("4k3/P7/8/8/8/8/8/4K3 w - - 0 1")
	standPat := Evaluate(pos)

	var nodes uint64
	score := Quiescence(context.Background(), pos, &nodes, Minimum, Maximum, 0)

	if score < standPat+500 {
		t.Errorf("quiescence score %d should be well above the static eval %d once the promotion is seen", score, standPat)
	}
}

// TestMoveScoreQueenPromotion checks that a quiet queen promotion is ordered
// like a big gain, while an underpromotion gets no bonus.
func TestMoveScoreQueenPromotion(t *testing.T) {
	pos := ParseFEN("4k3/P7/8/8/8/8/8/4K3 w - - 0 1")

	queen, capture := moveScore(*pos, NewPromotionMove(A7, A8, Queen))
	if queen <= 0 || capture {
		t.Errorf("quiet queen promotion: want positive score and capture=false, got %d, %v", queen, capture)
	}

	if knight, _ := moveScore(*pos, NewPromotionMove(A7, A8, Knight)); knight != 0 {
		t.Errorf("underpromotion: want score 0, got %d", knight)
	}
}

// TestOrderScoreLosingCaptureLast checks the ordering bands: a winning capture
// ranks above quiet moves, a capture that loses material by SEE ranks below all
// of them (even ones with no history or killer bonus), and the TT move is first.
func TestOrderScoreLosingCaptureLast(t *testing.T) {
	s := &SearchState{ctx: context.Background()}

	losing := ParseFEN("4k3/8/2p5/3p4/8/8/8/3QK3 w - - 0 1")
	quiet := s.orderScore(*losing, 0, NewMove(E1, F1), Move(0))
	bad := s.orderScore(*losing, 0, NewMove(D1, D5), Move(0))
	if bad >= quiet {
		t.Errorf("losing capture scored %d, must rank below a quiet move (%d)", bad, quiet)
	}
	if tt := s.orderScore(*losing, 0, NewMove(D1, D5), NewMove(D1, D5)); tt <= quiet {
		t.Errorf("TT move scored %d, must rank above everything (quiet %d)", tt, quiet)
	}

	winning := ParseFEN("4k3/8/8/3p4/8/8/8/3QK3 w - - 0 1")
	quiet = s.orderScore(*winning, 0, NewMove(E1, F1), Move(0))
	good := s.orderScore(*winning, 0, NewMove(D1, D5), Move(0))
	if good <= quiet {
		t.Errorf("winning capture scored %d, must rank above a quiet move (%d)", good, quiet)
	}
}

// repetitionFEN is a won KRvK position with a nonzero halfmove clock, so the
// repetition window covers the whole test history slices.
const repetitionFEN = "4k3/8/8/8/8/8/8/R3K3 w - - 10 30"

// TestTTCutoffSkippedForRepeatedPosition guards against the TT hiding a
// repetition: a stored score for a position that already occurred in the game
// knows nothing about the history, so it must not be returned as a cutoff --
// otherwise the search doesn't see that the opponent can complete a threefold.
// (In a real game this made the engine walk into a draw at +2.5.)
func TestTTCutoffSkippedForRepeatedPosition(t *testing.T) {
	pos := ParseFEN(repetitionFEN)
	h := pos.Hash()
	other := h ^ 0xABCD

	ttStore(h, 30, 5000, Exact, Move(0))
	defer func() { tTable[ttIndex(h)] = ttSlot{} }()

	s := &SearchState{ctx: context.Background(), rootIdx: 1}

	// h occurs only as the node itself: the stored score is trusted.
	if got := s.Negamax(pos, 3, 1, Minimum, Maximum, []uint64{other, h}, false, 0); got != 5000 {
		t.Fatalf("no earlier occurrence: want the TT score 5000, got %d", got)
	}

	// h already occurred before the root: no TT cutoff, the position is searched.
	ttStore(h, 30, 5000, Exact, Move(0))
	got := s.Negamax(pos, 3, 1, Minimum, Maximum, []uint64{h, other, h}, false, 0)
	if got == 5000 || got <= 0 {
		t.Fatalf("repeated position: want a real search score (winning, not the TT's 5000), got %d", got)
	}
}

// TestRepetitionScoring checks when a repeated position is scored as a draw:
// on any earlier occurrence inside the search, but for occurrences from before
// the root only on the third one; and never for the node right after a null move.
func TestRepetitionScoring(t *testing.T) {
	pos := ParseFEN(repetitionFEN)
	h := pos.Hash()
	a, b := h^1, h^2

	search := func(rootIdx int, nullMove bool, history []uint64) int {
		s := &SearchState{ctx: context.Background(), rootIdx: rootIdx}
		tTable[ttIndex(h)] = ttSlot{}
		defer func() { tTable[ttIndex(h)] = ttSlot{} }()
		return s.Negamax(pos, 3, 1, Minimum, Maximum, history, nullMove, 0)
	}

	if got := search(1, false, []uint64{a, h, a, h}); got != 0 {
		t.Errorf("repeated inside the search (root included): want draw 0, got %d", got)
	}
	if got := search(2, false, []uint64{h, a, b, h}); got == 0 {
		t.Errorf("one earlier occurrence before the root is not yet a draw, got 0")
	}
	if got := search(3, false, []uint64{h, a, h, b, h}); got != 0 {
		t.Errorf("third occurrence (two before the root): want draw 0, got %d", got)
	}
	if got := search(1, true, []uint64{a, h, a, h}); got == 0 {
		t.Errorf("node after a null move must not count as a repetition, got 0")
	}
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
