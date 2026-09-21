package engine

import (
	"context"
	"testing"
)

// TestLMRTable checks the shape of the table: nothing at the first moves, never
// falling as the depth or the move index grows, and small enough to be sane.
func TestLMRTable(t *testing.T) {
	for d := 1; d < len(lmrTable); d++ {
		if lmrTable[d][1] != 0 {
			t.Errorf("lmrTable[%d][1] = %d, want 0 (ln 1 = 0, the second move is searched in full)", d, lmrTable[d][1])
		}
		for i := 1; i < len(lmrTable[d]); i++ {
			r := lmrTable[d][i]
			if r < 0 || r > 8 {
				t.Errorf("lmrTable[%d][%d] = %d, want 0..8", d, i, r)
			}
			if i > 1 && r < lmrTable[d][i-1] {
				t.Errorf("lmrTable[%d][%d] = %d falls below the previous move's %d", d, i, r, lmrTable[d][i-1])
			}
			if d > 1 && r < lmrTable[d-1][i] {
				t.Errorf("lmrTable[%d][%d] = %d falls below the shallower search's %d", d, i, r, lmrTable[d-1][i])
			}
		}
	}

	// The examples in the comment on lmrTable.
	if got := lmrTable[6][10]; got != 2 {
		t.Errorf("lmrTable[6][10] = %d, want 2", got)
	}
	if got := lmrTable[10][20]; got != 3 {
		t.Errorf("lmrTable[10][20] = %d, want 3", got)
	}
}

func TestLMRReduction(t *testing.T) {
	newState := func() *SearchState { return &SearchState{ctx: context.Background()} }

	quiet := ParseFEN("rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1")
	knightMove := NewMove(G1, F3)
	const depth, i, ply = 10, 20, 4
	want := lmrTable[depth][i] // 3

	if got := newState().lmrReduction(*quiet, knightMove, depth, i, ply, false, false); got != want {
		t.Fatalf("plain quiet move: got %d, want the table value %d", got, want)
	}

	t.Run("never reduced", func(t *testing.T) {
		capture := ParseFEN("4k3/8/8/3p4/4P3/8/8/4K3 w - - 0 1")
		promotion := ParseFEN("4k3/P7/8/8/8/8/8/4K3 w - - 0 1")

		for name, tc := range map[string]int{
			"in check":        newState().lmrReduction(*quiet, knightMove, depth, i, ply, false, true),
			"first move":      newState().lmrReduction(*quiet, knightMove, depth, 0, ply, false, false),
			"depth 2":         newState().lmrReduction(*quiet, knightMove, 2, i, ply, false, false),
			"capture":         newState().lmrReduction(*capture, NewMove(E4, D5), depth, i, ply, false, false),
			"queen promotion": newState().lmrReduction(*promotion, NewPromotionMove(A7, A8, Queen), depth, i, ply, false, false),
			"underpromotion":  newState().lmrReduction(*promotion, NewPromotionMove(A7, A8, Knight), depth, i, ply, false, false),
		} {
			if tc != 0 {
				t.Errorf("%s: got %d, want 0", name, tc)
			}
		}
	})

	t.Run("one ply less for a PV node, a killer and a good history", func(t *testing.T) {
		if got := newState().lmrReduction(*quiet, knightMove, depth, i, ply, true, false); got != want-1 {
			t.Errorf("PV node: got %d, want %d", got, want-1)
		}

		s := newState()
		s.storeKiller(ply, knightMove)
		if got := s.lmrReduction(*quiet, knightMove, depth, i, ply, false, false); got != want-1 {
			t.Errorf("killer: got %d, want %d", got, want-1)
		}
		s.storeKiller(ply, NewMove(B1, C3)) // the first killer becomes the second
		if got := s.lmrReduction(*quiet, knightMove, depth, i, ply, false, false); got != want-1 {
			t.Errorf("second killer: got %d, want %d", got, want-1)
		}

		s = newState()
		s.historyHeu[White][G1][F3] = 100 * lmrHistoryThreshold
		if got := s.lmrReduction(*quiet, knightMove, depth, i, ply, false, false); got != want-1 {
			t.Errorf("history at the threshold: got %d, want %d", got, want-1)
		}
		s.historyHeu[White][G1][F3] = 100*lmrHistoryThreshold - 1
		if got := s.lmrReduction(*quiet, knightMove, depth, i, ply, false, false); got != want {
			t.Errorf("history just below the threshold: got %d, want %d", got, want)
		}

		s = newState()
		s.storeKiller(ply, knightMove)
		s.historyHeu[White][G1][F3] = 1 << 20
		if got := s.lmrReduction(*quiet, knightMove, depth, i, ply, true, false); got != max(0, want-3) {
			t.Errorf("all three: got %d, want %d", got, max(0, want-3))
		}
	})

	t.Run("bounds", func(t *testing.T) {
		s := newState()

		// At depth 3 the reduced search must keep one ply, however late the move.
		if got := s.lmrReduction(*quiet, knightMove, 3, 63, ply, false, false); got != 1 {
			t.Errorf("depth 3, move 63: got %d, want 1 (depth-2)", got)
		}
		// Indices and depths beyond the table are clamped, not out of range.
		if got := s.lmrReduction(*quiet, knightMove, 200, 500, ply, false, false); got != lmrTable[63][63] {
			t.Errorf("depth 200, move 500: got %d, want %d", got, lmrTable[63][63])
		}
		// A killer at a shallow node can't push the reduction below zero.
		s.storeKiller(ply, knightMove)
		if got := s.lmrReduction(*quiet, knightMove, 3, 2, ply, true, false); got != 0 {
			t.Errorf("negative reduction: got %d, want 0", got)
		}
	})
}
