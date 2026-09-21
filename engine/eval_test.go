package engine

import (
	"math/rand"
	"testing"
)

// mirrorPosition flips the board vertically and swaps the colors (and the
// side to move, castling rights and en passant square with them), so the
// result is the same game situation seen from the other side.
func mirrorPosition(pos *Position) *Position {
	m := &Position{EnPassant: NoSquare, FiftyMovesRule: pos.FiftyMovesRule}

	for sq := A1; sq <= H8; sq++ {
		pt := pos.PieceAt(sq)
		if pt == AllPieces {
			continue
		}
		c := White
		if pos.Colors[Black]&sq.BB() != 0 {
			c = Black
		}
		m.PutPiece(sq^56, c^1, pt)
	}

	m.SideToMove = pos.SideToMove ^ 1
	if pos.EnPassant != NoSquare {
		m.EnPassant = pos.EnPassant ^ 56
	}
	if pos.Castling&WhiteKingside != 0 {
		m.Castling |= BlackKingside
	}
	if pos.Castling&WhiteQueenside != 0 {
		m.Castling |= BlackQueenside
	}
	if pos.Castling&BlackKingside != 0 {
		m.Castling |= WhiteKingside
	}
	if pos.Castling&BlackQueenside != 0 {
		m.Castling |= WhiteQueenside
	}

	return m
}

// evalTestPositions calls visit for every position of a few reference perft
// trees, of some hand-picked endgames (passed, blocked and isolated pawns,
// bishop pairs, open files) and of seeded random games, which is what
// actually reaches middlegames and endgames with a full mix of pieces.
func evalTestPositions(t *testing.T, visit func(pos *Position)) {
	t.Helper()

	var walk func(pos *Position, depth int)
	walk = func(pos *Position, depth int) {
		visit(pos)
		if depth == 0 {
			return
		}

		var moves MoveList
		GenerateLegalMoves(*pos, pos.SideToMove, &moves)
		for _, m := range moves.Slice() {
			undo := pos.MakeMove(m)
			walk(pos, depth-1)
			pos.UnmakeMove(m, undo)
		}
	}

	fens := []string{
		StartFEN,
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
		"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1",
		"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8",
		"r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10",
	}
	for _, fen := range fens {
		walk(ParseFEN(fen), 2)
	}

	endgames := []string{
		"8/5pk1/6p1/3P4/8/1P4P1/5PK1/8 w - - 0 1",
		"8/P7/8/8/8/8/p7/k1K5 w - - 0 1",
		"8/3k4/8/3pP3/3P4/8/3K4/8 w - - 0 1",
		"4k3/8/3p4/3P4/8/8/PP3PPP/4K3 b - - 0 1",
		"2b1kb2/8/8/8/8/8/8/2B1KB2 w - - 0 1",
		"r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1",
		"4k3/1p6/8/P7/P7/8/8/4K3 w - - 0 1",
		"3rk3/3p4/8/8/8/8/3P4/3RK3 w - - 0 1",
	}
	for _, fen := range endgames {
		walk(ParseFEN(fen), 2)
	}

	rng := rand.New(rand.NewSource(1))
	for range 300 {
		pos := ParseFEN(StartFEN)
		for range 160 {
			var moves MoveList
			GenerateLegalMoves(*pos, pos.SideToMove, &moves)
			if len(moves.Slice()) == 0 {
				break
			}
			pos.MakeMove(moves.Slice()[rng.Intn(len(moves.Slice()))])
			visit(pos)
		}
	}
}

// TestEvaluateSymmetry checks that Evaluate has no color bias: the mirrored
// position (colors swapped, board flipped) must score exactly the negation.
func TestEvaluateSymmetry(t *testing.T) {
	n := 0
	evalTestPositions(t, func(pos *Position) {
		n++
		got, mirrored := Evaluate(pos), Evaluate(mirrorPosition(pos))
		if got != -mirrored {
			t.Fatalf("Evaluate = %d but mirrored position gives %d (want %d)", got, mirrored, -got)
		}
	})
	t.Logf("checked %d positions", n)
}

// TestTraceMatchesEvaluate checks the contract of Trace: summing its counts
// times the weights, tapered by phase, reproduces Evaluate exactly. It runs
// on the real weights and on random ones, because with the real weights most
// mg and eg values are equal and a mixed-up phase or a swapped mg/eg would go
// unnoticed.
func TestTraceMatchesEvaluate(t *testing.T) {
	saved := evalWeights
	defer func() { evalWeights = saved }()

	rng := rand.New(rand.NewSource(2))

	for run := range 4 {
		if run > 0 {
			for i := range evalWeights {
				evalWeights[i] = weight{mg: rng.Intn(401) - 200, eg: rng.Intn(401) - 200}
			}
		}

		n := 0
		evalTestPositions(t, func(pos *Position) {
			n++
			entries, phase := Trace(pos)
			if phase != gamePhase(pos) {
				t.Fatalf("phase = %d, want %d", phase, gamePhase(pos))
			}

			var mg, eg int
			seen := map[uint16]bool{}
			for _, e := range entries {
				if e.Count == 0 || seen[e.Index] {
					t.Fatalf("bad entry %+v (zero count or repeated index)", e)
				}
				seen[e.Index] = true
				mg += int(e.Count) * evalWeights[e.Index].mg
				eg += int(e.Count) * evalWeights[e.Index].eg
			}

			if got, want := (mg*phase+eg*(totalPhase-phase))/totalPhase, Evaluate(pos); got != want {
				t.Fatalf("run %d: trace gives %d, Evaluate gives %d", run, got, want)
			}
		})
		t.Logf("run %d: checked %d positions", run, n)
	}
}

// TestWeightNames checks that every weight has a distinct, non-empty name,
// which also catches gaps or overlaps between the blocks of the layout.
func TestWeightNames(t *testing.T) {
	seen := map[string]int{}
	for i := range NumWeights {
		name := WeightName(i)
		if name == "" {
			t.Fatalf("weight %d has no name", i)
		}
		if j, dup := seen[name]; dup {
			t.Fatalf("weights %d and %d are both called %q", j, i, name)
		}
		seen[name] = i
	}

	for _, tc := range []struct {
		idx  int
		want string
	}{
		{wMaterial + int(Queen), "material/queen"},
		{wPST + int(Knight)*64 + int(E4^56), "pst/knight/e4"},
		{wPST + int(King)*64, "pst/king/a8"},
		{wPassedBlocked + 5, "passed-blocked/rank6"},
		// Every scalar index must be its own weight: two constants sharing an
		// index would just make the vector shorter, and Evaluate and Trace
		// would agree with each other about the mistake.
		{wKnightMobility, "mobility/knight"},
		{wBishopMobility, "mobility/bishop"},
		{wRookMobility, "mobility/rook"},
		{wQueenMobility, "mobility/queen"},
		{wBishopPair, "bishop-pair"},
		{wRookOpenFile, "rook-open-file"},
		{wRookSemiOpenFile, "rook-semi-open-file"},
		{wPawnShield, "pawn-shield"},
		{wDoubledPawn, "doubled-pawn"},
		{wIsolatedPawn, "isolated-pawn"},
		{wPassed, "passed/rank1"},
		{wPassed + 7, "passed/rank8"},
		{wPassedBlocked, "passed-blocked/rank1"},
		{numWeights - 1, "passed-blocked/rank8"},
	} {
		if got := WeightName(tc.idx); got != tc.want {
			t.Errorf("WeightName(%d) = %q, want %q", tc.idx, got, tc.want)
		}
	}
}

// TestTunedWeightsKnown checks that every name in tunedWeights is a real
// weight: init applies the overrides by name, so a stale or misspelled entry
// (say, after a feature was renamed) would otherwise be silently ignored.
func TestTunedWeightsKnown(t *testing.T) {
	names := map[string]bool{}
	for i := range NumWeights {
		names[WeightName(i)] = true
	}
	for name := range tunedWeights {
		if !names[name] {
			t.Errorf("tunedWeights has %q, which is not a weight name", name)
		}
	}
}
