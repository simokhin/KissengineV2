package engine

import "testing"

// perftUnmake walks the move tree the same way perft does, but mutates pos
// in place via MakeMove/UnmakeMove instead of copying, verifying at every
// node that UnmakeMove restores the position exactly.
func perftUnmake(t *testing.T, pos *Position, depth int) uint64 {
	t.Helper()

	if depth == 0 {
		return 1
	}

	var nodes uint64

	moves := GenerateLegalMoves(*pos, pos.SideToMove)

	for _, move := range moves {
		beforeHash := pos.Hash()
		beforeFiftyMoves := pos.FiftyMovesRule

		undo := pos.MakeMove(move)
		nodes += perftUnmake(t, pos, depth-1)
		pos.UnmakeMove(move, undo)

		if pos.Hash() != beforeHash || pos.FiftyMovesRule != beforeFiftyMoves {
			t.Fatalf("UnmakeMove did not restore position for move %s", move.UCI())
		}
	}

	return nodes
}

func TestUnmakeMoveRoundTrip(t *testing.T) {
	pos := StartPos()

	want := []uint64{20, 400, 8902, 197281, 4865609}

	for depth, w := range want {
		got := perftUnmake(t, pos, depth+1)
		if got != w {
			t.Fatalf("depth %d: want %d, got %d", depth+1, w, got)
		}
	}
}
