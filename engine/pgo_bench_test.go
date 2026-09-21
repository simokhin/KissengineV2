package engine

import "testing"

// BenchmarkPGOProfile is the workload the compiler's profile-guided
// optimization is trained on (`make pgo` records a CPU profile of it into
// cmd/default.pgo, which `go build` picks up by itself). It should look like
// real play, so it mixes an opening, tactical and quiet middlegames and endgames
// at depths that each take a few seconds (about 8 s in all, so the profile has
// enough samples); it is not a benchmark to compare
// speeds with, use `go depth N` runs for that.
func BenchmarkPGOProfile(b *testing.B) {
	workload := []struct {
		fen   string
		depth int
	}{
		{StartFEN, 12},
		{"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", 10},
		{"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", 10},
		{"r1bq1rk1/pp2ppbp/2np1np1/8/3NP3/2N1BP2/PPPQ2PP/R3KB1R w KQ - 0 9", 10},
		{"r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10", 10},
		{"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 14},
		{"8/5pk1/6p1/3P4/8/1P4P1/5PK1/8 w - - 0 1", 18},
	}

	for b.Loop() {
		for _, w := range workload {
			pos := ParseFEN(w.fen)
			ClearHash()
			SearchDepth(*pos, w.depth, []uint64{pos.Hash()})
		}
	}
}
