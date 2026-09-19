package engine

import (
	"fmt"
	"testing"
)

// Reference positions and node counts from https://www.chessprogramming.org/Perft_Results
// (index i of nodes is the count at depth i+1). Beyond the start position these exercise
// castling, pins, en passant and promotions, which start-position perft never reaches.
func TestPerft(t *testing.T) {
	tests := []struct {
		name  string
		fen   string
		nodes []uint64
	}{
		{"starting position", StartFEN, []uint64{20, 400, 8902, 197281, 4865609}},
		{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", []uint64{48, 2039, 97862, 4085603}},
		{"position 3", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", []uint64{14, 191, 2812, 43238, 674624}},
		{"position 4", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1", []uint64{6, 264, 9467, 422333}},
		{"position 5", "rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8", []uint64{44, 1486, 62379, 2103487}},
		{"position 6", "r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10", []uint64{46, 2079, 89890, 3894594}},
	}

	for _, tt := range tests {
		pos := ParseFEN(tt.fen)

		for i, want := range tt.nodes {
			depth := i + 1

			t.Run(fmt.Sprintf("%s depth %d", tt.name, depth), func(t *testing.T) {
				if got := perft(*pos, depth); got != want {
					t.Errorf("want %d nodes, got %d nodes", want, got)
				}
			})
		}
	}
}
