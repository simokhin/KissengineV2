package engine

import (
	"bufio"
	"context"
	"os"
	"slices"
	"testing"
	"time"
)

// pollCtx is a context that gets cancelled after its Done channel has been
// polled a given number of times. Negamax polls it once per node, so it cuts a
// search off at a reproducible point, which a wall-clock timeout can't.
type pollCtx struct {
	context.Context
	polls, limit int
	done         chan struct{}
	closed       bool
}

func newPollCtx(limit int) *pollCtx {
	return &pollCtx{Context: context.Background(), limit: limit, done: make(chan struct{})}
}

func (c *pollCtx) Done() <-chan struct{} {
	c.polls++
	if c.polls >= c.limit && !c.closed {
		c.closed = true
		close(c.done)
	}
	return c.done
}

func (c *pollCtx) Err() error {
	if c.closed {
		return context.Canceled
	}
	return nil
}

// firstOpenings returns the first n positions of the saved opening set.
func firstOpenings(t *testing.T, n int) []*Position {
	t.Helper()

	f, err := os.Open("../testdata/openings-8ply.epd")
	if err != nil {
		t.Skip("opening set not available:", err)
	}
	defer f.Close()

	var out []*Position
	sc := bufio.NewScanner(f)
	for sc.Scan() && len(out) < n {
		out = append(out, ParseFEN(sc.Text()))
	}
	return out
}

func legalMoves(pos *Position) []Move {
	var list MoveList
	GenerateLegalMoves(*pos, pos.SideToMove, &list)
	return slices.Clone(list.Slice())
}

// TestCutOffIterationKeepsProvenBetterMove cuts a search off at many points
// inside an iteration whose best move differs from the previous one. Before, the
// cut-off iteration was thrown away whatever it had found; now a move that has
// already beaten the previous best at that depth is returned. Every cut has to
// give a legal move, and some of them have to give the new one.
func TestCutOffIterationKeepsProvenBetterMove(t *testing.T) {
	const maxDepth = 7

	changes, adopted, sameAsFinal, cuts := 0, 0, 0, 0
	for _, pos := range firstOpenings(t, 5) {
		history := []uint64{pos.Hash()}
		legal := legalMoves(pos)

		// An uncut search: the best move and the number of polls at the end of each depth.
		ClearHash()
		ctx := newPollCtx(1 << 60)
		bestAt, pollsAt := map[int]Move{}, map[int]int{}
		Search(ctx, *pos, history, maxDepth, func(info Info) {
			bestAt[info.Depth], pollsAt[info.Depth] = info.PV[0], ctx.polls
		})

		for d := 3; d <= maxDepth; d++ {
			if bestAt[d] == bestAt[d-1] {
				continue
			}
			changes++

			// Cut points inside iteration d: after the end of d-1, before the end of d.
			lo, hi := pollsAt[d-1]+1, pollsAt[d]-1
			for cut := lo; cut <= hi; cut += max(1, (hi-lo)/6) {
				ClearHash()
				got, info := Search(newPollCtx(cut), *pos, history, maxDepth, nil)
				cuts++

				if !slices.Contains(legal, got) {
					t.Fatalf("%v depth %d cut at %d: returned %v, which is not legal", pos.SideToMove, d, cut, got)
				}
				if info.Depth != d-1 {
					t.Fatalf("depth %d cut at %d: info.Depth = %d, want the last completed depth %d", d, cut, info.Depth, d-1)
				}
				if got != bestAt[d-1] {
					adopted++
					if got == bestAt[d] {
						sameAsFinal++
					}
				}
			}
		}
	}

	t.Logf("%d best-move changes, %d cut points, the cut-off iteration's move was used in %d (%d of them the move the finished iteration chose)", changes, cuts, adopted, sameAsFinal)
	if changes == 0 {
		t.Skip("no position in the sample changed its best move between depths")
	}
	if adopted == 0 {
		t.Errorf("no cut point returned a move from the cut-off iteration (%d cuts): the partial result is never used", cuts)
	}
}

// TestSingleLegalMoveReturnsAtOnce: with a deadline on the context and one legal
// move, Search returns it after depth 1 instead of using the clock; without a
// deadline (analysis) or with a depth limit it still searches.
func TestSingleLegalMoveReturnsAtOnce(t *testing.T) {
	var only *Position
	for _, fen := range []string{
		"7k/8/8/8/8/8/6q1/K7 w - - 0 1", "k7/8/8/8/8/8/1q6/K7 w - - 0 1", "8/8/8/8/8/2k5/1q6/K7 w - - 0 1",
		"6rk/8/8/8/8/8/5PPP/K5R1 b - - 0 1", "7k/6pp/8/8/8/8/1q6/K7 w - - 0 1", "r7/8/8/8/8/1k6/8/K7 w - - 0 1",
		"8/8/8/8/8/1k6/1r6/K7 w - - 0 1", "8/8/8/8/2k5/1q6/8/K7 w - - 0 1", "1r6/8/8/8/8/2k5/8/K7 w - - 0 1",
	} {
		if pos := ParseFEN(fen); len(legalMoves(pos)) == 1 {
			only = pos
			break
		}
	}
	if only == nil {
		t.Skip("no candidate position has exactly one legal move")
	}
	want := legalMoves(only)[0]
	history := []uint64{only.Hash()}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	got, info := Search(ctx, *only, history, 0, nil)
	if got != want || info.Depth != 1 || time.Since(start) > time.Second {
		t.Errorf("timed search: got %v at depth %d after %v, want %v at depth 1 at once", got, info.Depth, time.Since(start), want)
	}

	if _, info := Search(context.Background(), *only, history, 5, nil); info.Depth != 5 {
		t.Errorf("without a deadline the search stopped at depth %d, want 5", info.Depth)
	}
}
