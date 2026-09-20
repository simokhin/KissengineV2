package engine

import (
	"context"
	"time"
)

// MaxDepth is the deepest iteration Search will run. Killer tables are sized
// for it (with room for check extensions), so it is also what "no depth limit"
// means.
const MaxDepth = 64

// maxPVLength caps how many plies of principal variation are reported.
const maxPVLength = 32

// Info describes the last completed iteration of a search.
type Info struct {
	Depth int
	Score int
	Nodes uint64 // total over all iterations so far, cut-off ones included
	Time  time.Duration
	PV    []Move
}

// Search performs iterative-deepening negamax with aspiration windows until
// ctx is done or maxDepth (0 means MaxDepth) has been completed, and returns
// the best move of the last completed iteration with that iteration's Info.
// onInfo, if not nil, is called after every completed iteration.
//
// Depth 1 always seeds the best move with a real legal move (even under
// extreme time pressure), and later depths only overwrite it once they finish
// completely -- a depth cut short by ctx is discarded rather than allowed to
// replace a good, fully-searched result with a worse, half-searched one. If
// the root has no legal moves the returned move is Move(0).
func Search(ctx context.Context, pos Position, history []uint64, maxDepth int, onInfo func(Info)) (Move, Info) {
	if maxDepth <= 0 || maxDepth > MaxDepth {
		maxDepth = MaxDepth
	}

	start := time.Now()
	s := &SearchState{ctx: ctx}

	var info Info

	// complete records a finished iteration and reports it.
	complete := func(move Move, depth, score int) {
		info.Depth = depth
		info.Score = score
		info.Time = time.Since(start)
		info.PV = principalVariation(pos, move, maxPVLength)

		if onInfo != nil {
			onInfo(info)
		}
	}

	bestMove, nodes, bestScore := BestMove(s, pos, 1, history, Minimum, Maximum)
	info.Nodes = nodes
	info.Score = bestScore
	if bestMove == 0 {
		return 0, info // mate or stalemate: nothing to search
	}
	if ctx.Err() == nil {
		complete(bestMove, 1, bestScore)
	}

	windowSize := 50
	alpha, beta := bestScore-windowSize, bestScore+windowSize

	for depth := 2; depth <= maxDepth; depth++ {
		if ctx.Err() != nil {
			break
		}

		move, nodes, score := BestMove(s, pos, depth, history, alpha, beta)
		info.Nodes += nodes

		if ctx.Err() != nil {
			break
		}

		if score <= alpha || score >= beta {
			alpha, beta = Minimum, Maximum
			depth--
			continue
		}

		bestMove = move
		bestScore = score
		complete(move, depth, score)

		alpha, beta = score-windowSize, score+windowSize
	}

	info.Time = time.Since(start)

	return bestMove, info
}

// SearchTimed searches until timeLimit expires, returning the best move, the
// total node count, the deepest depth fully searched and its score.
func SearchTimed(pos Position, timeLimit time.Duration, history []uint64) (Move, uint64, int, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeLimit)
	defer cancel()

	move, info := Search(ctx, pos, history, 0, nil)

	return move, info.Nodes, info.Depth, info.Score
}

// SearchDepth searches to a fixed depth, returning the best move, the total
// node count, the depth reached and its score.
func SearchDepth(pos Position, maxDepth int, history []uint64) (Move, uint64, int, int) {
	move, info := Search(context.Background(), pos, history, maxDepth, nil)

	return move, info.Nodes, info.Depth, info.Score
}

// principalVariation returns first followed by the continuation stored in the
// transposition table: each position's best move, as long as the entry exists,
// the move is legal there and the position hasn't been seen on the line yet.
// It can end early where an entry was overwritten, which is fine for display.
func principalVariation(pos Position, first Move, maxLen int) []Move {
	pv := []Move{first}
	seen := []uint64{pos.Hash()}

	pos.MakeMove(first)

	for len(pv) < maxLen {
		hash := pos.Hash()
		entry, found := ttProbe(hash)
		if !found || entry.bestMove == 0 {
			break
		}

		repeated := false
		for _, h := range seen {
			if h == hash {
				repeated = true
				break
			}
		}
		if repeated {
			break
		}

		var list MoveList
		GenerateLegalMoves(pos, pos.SideToMove, &list)
		legal := false
		for _, m := range list.Slice() {
			if m == entry.bestMove {
				legal = true
				break
			}
		}
		if !legal {
			break
		}

		seen = append(seen, hash)
		pv = append(pv, entry.bestMove)
		pos.MakeMove(entry.bestMove)
	}

	return pv
}
