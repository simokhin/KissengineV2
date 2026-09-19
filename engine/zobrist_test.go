package engine

import "testing"

// walkHashCheck walks the legal move tree to the given depth, verifying at
// every node that the incrementally-maintained Hash() matches an
// independent hashFromScratch() recomputation -- catching bugs where a
// state change (e.g. castling rights) is missed by both MakeMove and
// UnmakeMove symmetrically, which a plain make/unmake round-trip check
// would not detect.
func walkHashCheck(t *testing.T, pos *Position, depth int) {
	t.Helper()

	if got, want := pos.Hash(), pos.hashFromScratch(); got != want {
		t.Fatalf("Hash() = %d, want %d (hashFromScratch)", got, want)
	}

	if depth == 0 {
		return
	}

	for _, move := range GenerateLegalMoves(*pos, pos.SideToMove) {
		undo := pos.MakeMove(move)
		walkHashCheck(t, pos, depth-1)
		pos.UnmakeMove(move, undo)
	}
}

func TestHashMatchesFromScratch(t *testing.T) {
	positions := []struct {
		name string
		fen  string
	}{
		{"startpos", StartFEN},
		{"Kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"},
		{"Position3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1"},
		{"Position4", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1"},
		{"Position5", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8"},
	}

	for _, p := range positions {
		t.Run(p.name, func(t *testing.T) {
			pos := ParseFEN(p.fen)
			walkHashCheck(t, pos, 3)
		})
	}
}
