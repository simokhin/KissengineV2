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

// aspirationDelta is the half-width of the first aspiration window tried
// after every completed iteration, and the starting size of the widening
// step on a failure (doubling with every further failure at that depth).
// Untuned; the value carries over from the previous fixed +/-50 window.
const aspirationDelta = 50

// Info describes the last completed iteration of a search. The move Search
// returns can differ from PV[0] when the search was cut off during a later
// iteration that had already found a better one (see Search).
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
// extreme time pressure), and later depths overwrite it once they finish
// completely. A depth cut short by ctx is not allowed to replace a good,
// fully-searched result with a half-searched one, with one exception: a root
// move that has already beaten every move tried before it at that depth (the
// previous best is searched first), by searches that ran to completion, is a
// better move than the previous depth's, so it is returned instead. That saves
// the work of an iteration that was mostly done when time ran out. If the root
// has no legal moves the returned move is Move(0), and with a deadline on ctx
// and only one legal move Search returns it at once: there is nothing to
// choose, so thinking would only spend clock.
func Search(ctx context.Context, pos Position, history []uint64, maxDepth int, onInfo func(Info)) (Move, Info) {
	return SearchSoft(ctx, pos, history, maxDepth, 0, onInfo)
}

// SearchSoft is Search with time management. The deadline of ctx is the hard
// limit: the search never runs past it. soft (0 for none; it needs a deadline on
// ctx, and only means something below it) is the time the search aims to spend.
// At the soft limit a settled position stops, and iterations that the last ones
// predict won't finish in time aren't started, so the clock keeps the rest for
// later moves. An unsettled one (the best move just changed, the score fell, the
// first move failed low or another took over) carries on towards the hard limit,
// where the work of a cut-off iteration is still used, see Search.
func SearchSoft(ctx context.Context, pos Position, history []uint64, maxDepth int, soft time.Duration, onInfo func(Info)) (Move, Info) {
	if maxDepth <= 0 || maxDepth > MaxDepth {
		maxDepth = MaxDepth
	}

	start := time.Now()

	hardEnd, timed := ctx.Deadline()
	hard := time.Duration(0)
	if timed {
		hard = hardEnd.Sub(start)
	}
	if !timed || soft >= hard {
		soft = 0 // nothing to manage: no deadline, or the soft limit is the hard one
	}

	s := &SearchState{ctx: ctx}

	// The soft limit: when it comes, stop unless the position is unsettled, in
	// which case look again shortly (the hard deadline stops the search anyway).
	// It needs a context of its own to cancel; without a soft limit the caller's
	// is used as it is.
	if soft > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		s.ctx = ctx

		go func() {
			timer := time.NewTimer(soft)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					if !s.unstable.Load() {
						cancel()
						return
					}
					timer.Reset(softRecheck)
				}
			}
		}()
	}

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

	if timed {
		var legal MoveList
		GenerateLegalMoves(pos, pos.SideToMove, &legal)
		if len(legal.Slice()) == 1 {
			info.Time = time.Since(start)
			return bestMove, info
		}
	}

	// The move and score of the last completed iteration, which bestMove can run
	// ahead of (a move that failed high is taken as best before its exact score).
	lastMove, lastScore := bestMove, bestScore
	s.prevScore, s.havePrev = lastScore, true

	delta := aspirationDelta
	alpha, beta := max(bestScore-delta, Minimum), min(bestScore+delta, Maximum)

	// The times of the last two completed iterations, for the prediction of the
	// next one. An iteration that is searched again with a wider window keeps its
	// start time, so the repeat counts as part of it.
	var lastIteration, previousIteration time.Duration
	iterationStart := time.Now()
	repeating := false

	for depth := 2; depth <= maxDepth; depth++ {
		if ctx.Err() != nil {
			break
		}

		if !repeating {
			if soft > 0 && lastIteration > 0 && !wantsIteration(time.Since(start), lastIteration, previousIteration, soft, hard, s.unstable.Load()) {
				break
			}
			iterationStart = time.Now()
		}

		move, nodes, score := BestMove(s, pos, depth, history, alpha, beta)
		info.Nodes += nodes

		if ctx.Err() != nil {
			if s.rootPartial != 0 {
				bestMove = s.rootPartial
			}
			break
		}

		if score <= alpha || score >= beta {
			if score >= beta {
				bestMove = move // beat the window: better than the previous best, though not yet exactly scored
			}

			// Widen only the bound that failed, by a delta that doubles on
			// every further failure at this depth; the other bound is left
			// alone, since nothing said it was wrong. max/min clamp it open
			// to (Minimum, Maximum) after enough doublings, rather than
			// jumping there on the first failure the way a fixed window
			// re-searched at full width would.
			delta *= 2
			if score <= alpha {
				alpha = max(score-delta, Minimum)
			} else {
				beta = min(score+delta, Maximum)
			}
			depth--
			repeating = true
			continue
		}
		repeating = false

		s.unstable.Store(looksUnstable(move, score, lastMove, lastScore))
		lastMove, lastScore = move, score
		s.prevScore = score

		previousIteration, lastIteration = lastIteration, time.Since(iterationStart)

		bestMove = move
		complete(move, depth, score)

		delta = aspirationDelta
		alpha, beta = max(score-delta, Minimum), min(score+delta, Maximum)
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
