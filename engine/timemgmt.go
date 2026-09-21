package engine

import "time"

// Time management, in the terms of SearchSoft: the soft limit is the time the
// search aims to spend on the move, the hard limit (the deadline of the context)
// is the most it may spend. The search stops at the soft limit when the position
// looks settled and carries on towards the hard limit when it doesn't, since an
// unsettled root is where more time changes the move.

const (
	// unstableDrop is how far (in centipawns) the root score has to fall below the
	// previous iteration's for the position to count as unsettled.
	unstableDrop = 30

	// An iteration costs a multiple of the one before it: the growth is measured
	// from the last two, and kept within these bounds so that one odd iteration
	// (the first ones after a move the transposition table already knows are
	// nearly free) doesn't make the prediction absurd.
	minIterationGrowth = 2.0
	maxIterationGrowth = 6.0

	// softRecheck is how often a running iteration past the soft limit re-checks
	// whether the position has settled.
	softRecheck = 20 * time.Millisecond
)

// iterationGrowth is the factor by which the next iteration is expected to cost
// more than the last, given the times of the last two (prev is 0 if there was
// only one).
func iterationGrowth(last, prev time.Duration) float64 {
	if prev <= 0 || last <= 0 {
		return 3
	}
	return min(max(float64(last)/float64(prev), minIterationGrowth), maxIterationGrowth)
}

// wantsIteration reports whether to start another iteration, after elapsed time
// in which the last iteration took last and the one before it prev.
//
// A settled position starts one only if the prediction (the last iteration's
// time times its growth) says it will finish within the soft limit; time not
// spent stays on the clock for later moves. An unsettled one starts as long as
// less than half of the hard limit is used: the iteration may not finish, but
// what it finds by the deadline is still used (see Search).
func wantsIteration(elapsed, last, prev, soft, hard time.Duration, unstable bool) bool {
	if unstable {
		return elapsed < hard/2
	}

	expected := time.Duration(float64(last) * iterationGrowth(last, prev))
	return elapsed+expected <= soft
}

// looksUnstable reports whether an iteration that ended with the given move and
// score, after one that ended with prevMove and prevScore, leaves the position
// unsettled: the best move changed, or the score fell by unstableDrop or more.
// Mate scores are not compared; they say nothing about a drop in cp.
func looksUnstable(move Move, score int, prevMove Move, prevScore int) bool {
	if move != prevMove {
		return true
	}
	if isMateScore(score) || isMateScore(prevScore) {
		return false
	}
	return score <= prevScore-unstableDrop
}

func isMateScore(score int) bool {
	return score >= MateValue-1000 || score <= -(MateValue-1000)
}
