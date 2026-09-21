package engine

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestIterationGrowth(t *testing.T) {
	const ms = time.Millisecond
	for _, tc := range []struct {
		last, prev time.Duration
		want       float64
	}{
		{40 * ms, 0, 3},         // only one iteration to go by
		{0, 10 * ms, 3},         // nothing measured
		{12 * ms, 10 * ms, 2},   // barely grew: kept at the lower bound
		{40 * ms, 10 * ms, 4},   // the ratio itself
		{200 * ms, 10 * ms, 6},  // an outlier: kept at the upper bound
		{25 * ms, 10 * ms, 2.5}, // the ratio itself
	} {
		if got := iterationGrowth(tc.last, tc.prev); got != tc.want {
			t.Errorf("iterationGrowth(%v, %v) = %v, want %v", tc.last, tc.prev, got, tc.want)
		}
	}
}

func TestWantsIteration(t *testing.T) {
	const ms = time.Millisecond

	// Settled: another iteration only if the prediction fits the soft limit.
	// Last took 40 ms after 15 ms: growth 2.67, so the next is expected at ~107 ms.
	for _, tc := range []struct {
		name          string
		elapsed, soft time.Duration
		want          bool
	}{
		{"fits", 100 * ms, 300 * ms, true},
		{"fits exactly", 100 * ms, 100*ms + 40*ms*8/3, true},
		{"does not fit", 100 * ms, 200 * ms, false},
		{"soft limit already passed", 400 * ms, 300 * ms, false},
	} {
		if got := wantsIteration(tc.elapsed, 40*ms, 15*ms, tc.soft, 1000*ms, false); got != tc.want {
			t.Errorf("settled, %s: got %v, want %v", tc.name, got, tc.want)
		}
	}

	// Unsettled: go on while less than half of the hard limit is used, whatever
	// the prediction says (what a cut-off iteration finds is still used).
	for _, tc := range []struct {
		elapsed time.Duration
		want    bool
	}{
		{100 * ms, true},
		{499 * ms, true},
		{500 * ms, false},
		{900 * ms, false},
	} {
		if got := wantsIteration(tc.elapsed, 40*ms, 15*ms, 300*ms, 1000*ms, true); got != tc.want {
			t.Errorf("unsettled at %v of a 1000 ms hard limit: got %v, want %v", tc.elapsed, got, tc.want)
		}
	}
}

func TestLooksUnstable(t *testing.T) {
	a, b := NewMove(E2, E4), NewMove(D2, D4)

	for _, tc := range []struct {
		name      string
		move      Move
		score     int
		prevMove  Move
		prevScore int
		want      bool
	}{
		{"same move, same score", a, 20, a, 20, false},
		{"same move, better", a, 60, a, 20, false},
		{"same move, dropped by 29", a, -9, a, 20, false},
		{"same move, dropped by 30", a, -10, a, 20, true},
		{"different move, same score", b, 20, a, 20, true},
		{"mate now", a, MateValue - 5, a, 20, false},
		{"was mate, no longer", a, 20, a, MateValue - 5, false},
		{"mated now", a, -(MateValue - 4), a, 20, false},
	} {
		if got := looksUnstable(tc.move, tc.score, tc.prevMove, tc.prevScore); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestSearchSoftRespectsLimits checks SearchSoft against the clock, with wide
// margins so that it doesn't flake: the search never runs past the hard limit,
// always returns a legal move, and in most positions stops well before the hard
// limit (it can run on in a position that stays unsettled).
func TestSearchSoftRespectsLimits(t *testing.T) {
	const (
		soft = 40 * time.Millisecond
		hard = 400 * time.Millisecond
	)

	early, total := 0, 0
	for _, pos := range firstOpenings(t, 5) {
		history := []uint64{pos.Hash()}

		ClearHash()
		ctx, cancel := context.WithTimeout(context.Background(), hard)
		start := time.Now()
		move, _ := SearchSoft(ctx, *pos, history, 0, soft, nil)
		elapsed := time.Since(start)
		cancel()

		if !slices.Contains(legalMoves(pos), move) {
			t.Fatalf("returned %v, which is not legal", move)
		}
		if elapsed > hard+150*time.Millisecond {
			t.Errorf("search took %v, past the %v hard limit", elapsed, hard)
		}

		total++
		if elapsed < hard*3/4 {
			early++
		}
	}

	if early*2 < total {
		t.Errorf("the soft limit ended the search well before the hard one in only %d of %d positions", early, total)
	}
}

// TestSearchSoftIgnoredWithoutDeadline: with no deadline there is no hard limit
// for a soft one to be soft against, so a depth-limited search runs to its depth.
func TestSearchSoftIgnoredWithoutDeadline(t *testing.T) {
	pos := ParseFEN(StartFEN)

	_, info := SearchSoft(context.Background(), *pos, []uint64{pos.Hash()}, 6, time.Millisecond, nil)
	if info.Depth != 6 {
		t.Errorf("reached depth %d, want 6", info.Depth)
	}
}

// TestSoftTimerStopsARunningIteration: after a warm-up search the early
// iterations of the next one are almost free (the transposition table knows the
// position), so the prediction lets the search start an iteration that turns out
// to cost far more. The soft limit then has to stop it while it runs, because the
// prediction can't; the position is quiet and its best move stable, so nothing
// argues for more time.
func TestSoftTimerStopsARunningIteration(t *testing.T) {
	pos := ParseFEN("r1bq1rk1/pp2ppbp/2np1np1/8/3NP3/2N1BP2/PPPQ2PP/R3KB1R w KQ - 0 9")
	history := []uint64{pos.Hash()}

	ClearHash()
	warm, _, _, _ := SearchDepth(*pos, 10, history)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	move, info := SearchSoft(ctx, *pos, history, 0, 60*time.Millisecond, nil)
	elapsed := time.Since(start)

	// With the timer this takes ~60 ms (depth 10); without it the depth-11
	// iteration runs to its end, ~270 ms.
	if elapsed > 180*time.Millisecond {
		t.Errorf("the search ran %v (depth %d) with a 60 ms soft limit and a 3 s hard one, the soft limit didn't stop it", elapsed, info.Depth)
	}
	if move != warm {
		t.Logf("best move %v after the soft stop, %v from the warm-up search (only a note: this depends on how far it got)", move, warm)
	}
}
