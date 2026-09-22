package engine

import "testing"

// TestNNUEEvaluateKnownWeights hand-derives the expected output through
// every quantization step (accumulator -> L1 -> L2 -> Out -> centipawns),
// with weights chosen so no intermediate layer clips and the arithmetic
// stays easy to verify by hand. This exists because NNUEEvaluate once had a
// real bug — an extra spurious division by nnueWeightScale (QB) on top of
// nnueClipMax (QA) — caught only by comparing held-out loss against
// bullet's own reported training loss (tools/nnueval), which nothing in
// this file would have caught: TestNNUEForward only checks determinism and
// that flipping side to move changes the output, and the other tests here
// never look at absolute magnitude, so a wrong constant divisor sailed
// through all of them.
func TestNNUEEvaluateKnownWeights(t *testing.T) {
	// Zero feature weights make the accumulator equal the bias alone,
	// regardless of position or perspective — the cheapest way to get a
	// fully deterministic input into the rest of the network.
	savedFeatureWeights := nnueFeatureWeights
	savedFeatureBias := nnueFeatureBias
	savedL1Weights := nnueL1Weights
	savedL1Bias := nnueL1Bias
	savedL2Weights := nnueL2Weights
	savedL2Bias := nnueL2Bias
	savedOutWeights := nnueOutWeights
	savedOutBias := nnueOutBias
	t.Cleanup(func() {
		nnueFeatureWeights = savedFeatureWeights
		nnueFeatureBias = savedFeatureBias
		nnueL1Weights = savedL1Weights
		nnueL1Bias = savedL1Bias
		nnueL2Weights = savedL2Weights
		nnueL2Bias = savedL2Bias
		nnueOutWeights = savedOutWeights
		nnueOutBias = savedOutBias
	})

	nnueFeatureWeights = [nnueFeatures][nnueHidden]int16{}
	for i := range nnueFeatureBias {
		nnueFeatureBias[i] = 10 // accumulator = 10 for every hidden unit
	}
	// input[i] = clippedReLU(10) = 10, unclipped (0 <= 10 <= 127).

	for i := range nnueL1Weights {
		for j := range nnueL1Weights[i] {
			nnueL1Weights[i][j] = 1
		}
	}
	nnueL1Bias = [nnueL1Size]int32{}
	// sum = 512 inputs * 10 * weight 1 = 5120; l1out = clippedReLU(5120/64) = 80.

	for i := range nnueL2Weights {
		for j := range nnueL2Weights[i] {
			nnueL2Weights[i][j] = 1
		}
	}
	nnueL2Bias = [nnueL2Size]int32{}
	// sum = 32 inputs * 80 * weight 1 = 2560; l2out = clippedReLU(2560/64) = 40.

	for i := range nnueOutWeights {
		nnueOutWeights[i] = 1
	}
	nnueOutBias = 0
	// sum = 32 inputs * 40 * weight 1 = 1280; nnueForward() = 1280/64 = 20.
	// NNUEEvaluate = 20 * 400 / 127 = 8000/127 = 62 (integer division).

	pos := ParseFEN(StartFEN)
	if got := pos.nnueForward(); got != 20 {
		t.Fatalf("nnueForward() = %d, want 20", got)
	}

	const want = 62
	if got := pos.NNUEEvaluate(); got != want {
		t.Errorf("White to move: NNUEEvaluate() = %d, want %d", got, want)
	}

	pos.SideToMove = Black
	if got := pos.NNUEEvaluate(); got != -want {
		t.Errorf("Black to move: NNUEEvaluate() = %d, want %d", got, -want)
	}
}

func TestHalfKAIndexKnownCases(t *testing.T) {
	tests := []struct {
		name        string
		perspective Color
		kingSq      Square
		pieceSq     Square
		pt          PieceType
		pieceColor  Color
		want        int
	}{
		// White perspective, own white pawn on A1, king on A1: no mirroring,
		// own block (offset 0), pawn code 0 -> everything zero.
		{"white-own-pawn-a1-king-a1", White, A1, A1, Pawn, White, 0},
		// Same square/king but an enemy (black) piece: enemy block starts
		// at nnueColorBlock (384).
		{"white-enemy-pawn-a1-king-a1", White, A1, A1, Pawn, Black, nnueColorBlock},
		// Black perspective mirrors squares vertically (^56): king a1 -> a8,
		// piece a1 -> a8, own piece (black).
		{"black-own-pawn-a1-king-a1", Black, A1, A1, Pawn, Black, int(A8)*2*nnueColorBlock + int(A8)},
		// Unlike HalfKP, the king itself is now a feature: own white king
		// on e1, as seen from White's own perspective. E1 is file e (index
		// 4) of rank 1, i.e. square 4; bulletPieceCode[King] = 5.
		{"white-own-king-e1", White, E1, E1, King, White, int(E1)*2*nnueColorBlock + 64*5 + int(E1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := halfKAIndex(tt.perspective, tt.kingSq, tt.pieceSq, tt.pt, tt.pieceColor); got != tt.want {
				t.Errorf("halfKAIndex() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClippedReLU(t *testing.T) {
	tests := []struct {
		name string
		x    int32
		want int32
	}{
		{"negative clamps to 0", -50, 0},
		{"zero stays 0", 0, 0},
		{"below ceiling passes through", 100, 100},
		{"at ceiling passes through", nnueClipMax, nnueClipMax},
		{"above ceiling clamps to ceiling", nnueClipMax + 1, nnueClipMax},
		{"far above ceiling clamps to ceiling", 1_000_000, nnueClipMax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clippedReLU(tt.x); got != tt.want {
				t.Errorf("clippedReLU(%d) = %d, want %d", tt.x, got, tt.want)
			}
		})
	}
}

func TestNNUEForward(t *testing.T) {
	pos := ParseFEN("r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1")

	got1 := pos.nnueForward()
	got2 := pos.nnueForward()
	if got1 != got2 {
		t.Fatalf("nnueForward() not deterministic: %d then %d", got1, got2)
	}

	// Flipping whose move it is swaps which accumulator is "own" vs
	// "opponent" in the input layer, so with generic (non-symmetric)
	// placeholder weights the output should generally change.
	pos.SideToMove ^= 1
	if flipped := pos.nnueForward(); flipped == got1 {
		t.Errorf("nnueForward() unchanged after flipping side to move: both %d", got1)
	}
}

func walkAccumulatorCheck(t *testing.T, pos *Position, depth int) {
	t.Helper()

	for persp := White; persp <= Black; persp++ {
		got := *pos.Accumulator(persp)
		want := pos.accumulatorFromScratch(persp)
		if got != want {
			t.Fatalf("Accumulator(%v) does not match accumulatorFromScratch", persp)
		}
	}

	if depth == 0 {
		return
	}

	var moveList MoveList
	GenerateLegalMoves(*pos, pos.SideToMove, &moveList)

	for _, move := range moveList.Slice() {
		undo := pos.MakeMove(move)
		walkAccumulatorCheck(t, pos, depth-1)
		pos.UnmakeMove(move, undo)
	}
}

func TestAccumulatorMatchesFromScratch(t *testing.T) {
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
			walkAccumulatorCheck(t, pos, 3)
		})
	}
}
