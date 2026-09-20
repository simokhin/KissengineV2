package engine

import (
	"context"
	"testing"
	"time"
)

func isLegal(pos Position, m Move) bool {
	var list MoveList
	GenerateLegalMoves(pos, pos.SideToMove, &list)
	for _, lm := range list.Slice() {
		if lm == m {
			return true
		}
	}
	return false
}

// TestSearchReportsEveryDepth: onInfo fires once per completed depth, in
// order, each with a principal variation that is a legal line from the root and
// starts with the move Search finally returns.
func TestSearchReportsEveryDepth(t *testing.T) {
	ClearHash()
	defer ClearHash()

	pos := StartPos()
	history := []uint64{pos.Hash()}

	var infos []Info
	move, final := Search(context.Background(), *pos, history, 4, func(i Info) { infos = append(infos, i) })

	if len(infos) != 4 {
		t.Fatalf("got %d info reports, want 4 (depths 1-4)", len(infos))
	}
	for i, info := range infos {
		if info.Depth != i+1 {
			t.Errorf("report %d has depth %d, want %d", i, info.Depth, i+1)
		}
		if len(info.PV) == 0 {
			t.Fatalf("depth %d: empty PV", info.Depth)
		}

		p := *pos
		for _, m := range info.PV {
			if !isLegal(p, m) {
				t.Fatalf("depth %d: PV move %s is not legal in the line", info.Depth, m.UCI())
			}
			p.MakeMove(m)
		}
	}

	last := infos[len(infos)-1]
	if last.PV[0] != move || final.Depth != 4 {
		t.Errorf("returned move %s depth %d, want PV head %s depth 4", move.UCI(), final.Depth, last.PV[0].UCI())
	}
	if infos[3].Nodes <= infos[0].Nodes {
		t.Errorf("node count should grow across depths: %d then %d", infos[0].Nodes, infos[3].Nodes)
	}
}

// TestSearchStopsOnCancel: cancelling the context ends an unlimited search
// promptly with a legal move from a completed iteration.
func TestSearchStopsOnCancel(t *testing.T) {
	ClearHash()
	defer ClearHash()

	pos := StartPos()
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	move, info := Search(ctx, *pos, []uint64{pos.Hash()}, 0, nil)

	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("search took %v after a 150ms deadline", elapsed)
	}
	if !isLegal(*pos, move) {
		t.Errorf("returned move %s is not legal", move.UCI())
	}
	if info.Depth < 1 {
		t.Errorf("no completed depth reported")
	}
}

// TestSearchNoLegalMoves: a mated root returns no move instead of searching.
func TestSearchNoLegalMoves(t *testing.T) {
	pos := ParseFEN("rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3") // fool's mate
	move, info := Search(context.Background(), *pos, []uint64{pos.Hash()}, 5, nil)

	if move != 0 {
		t.Errorf("got move %s in a mated position, want none", move.UCI())
	}
	if info.Score > -(MateValue - 1000) {
		t.Errorf("score %d, want a mated score", info.Score)
	}
}

// TestPrincipalVariationFollowsTT: the line continues through legal TT moves
// and stops at a missing entry or an illegal stored move.
func TestPrincipalVariationFollowsTT(t *testing.T) {
	ClearHash()
	defer ClearHash()

	pos := *StartPos()
	first := NewMove(E2, E4)

	after := pos
	after.MakeMove(first)
	h := after.Hash()

	if pv := principalVariation(pos, first, 8); len(pv) != 1 {
		t.Fatalf("no TT entry: PV length %d, want 1", len(pv))
	}

	ttStore(h, 5, 0, Exact, NewMove(E2, E4)) // no such pawn any more: illegal for Black
	if pv := principalVariation(pos, first, 8); len(pv) != 1 {
		t.Fatalf("illegal TT move: PV length %d, want 1", len(pv))
	}

	ttStore(h, 5, 0, Exact, NewMove(E7, E5))
	pv := principalVariation(pos, first, 8)
	if len(pv) != 2 || pv[1] != NewMove(E7, E5) {
		t.Fatalf("legal TT move: PV %v, want e2e4 e7e5", pv)
	}

	if pv := principalVariation(pos, first, 1); len(pv) != 1 {
		t.Errorf("maxLen 1 gave PV length %d", len(pv))
	}
}
