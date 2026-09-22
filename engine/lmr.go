package engine

import "math"

// lmrTable[depth][moveIndex] is how many plies late move reductions take off
// the search of a move: it grows with the depth (there is more to lose from a
// shallow search of a hopeless move when there is more depth to spare) and with
// how late the move comes in the ordering (moves that are tried late rarely
// matter). The usual formula, 0.75 + ln(depth)*ln(i)/2.25, gives e.g. 2 plies
// at depth 6 for the tenth move and 3 at depth 10 for the twentieth.
var lmrTable [64][64]int

func init() {
	for d := 1; d < len(lmrTable); d++ {
		for i := 1; i < len(lmrTable[d]); i++ {
			lmrTable[d][i] = int(0.75 + math.Log(float64(d))*math.Log(float64(i))/2.25)
		}
	}
}

// lmrHistoryThreshold is the history bonus (0..39, see historyBonus) from which
// a move counts as one that keeps causing cutoffs and is reduced one ply less.
const lmrHistoryThreshold = 20

// historyBonus is the history heuristic's contribution to a quiet move's
// ordering score, capped below the killer and countermove bonuses (60/50/40).
func (s *SearchState) historyBonus(side Color, m Move) int {
	return min(s.historyHeu[side][m.From()][m.To()]/100, 39)
}

// isKiller reports whether m is one of the killer moves stored for ply.
func (s *SearchState) isKiller(ply int, m Move) bool {
	return ply < len(s.killers) && (m == s.killers[ply][0] || m == s.killers[ply][1])
}

// lmrReduction returns by how many plies to shorten the search of move m, the
// i-th in the ordering at a node of the given depth (0 = search it in full).
// It has to be called before the move is made: it looks at what the move
// captures, and afterwards the target square holds the mover.
//
// Never reduced: the first move, moves at a node in check (evasions), captures
// and promotions, and everything at depth < 3. Otherwise the table value, one
// ply less for a move in the principal variation (pvNode), a killer, or a move
// with a high history score, and never more than depth-2, so that the reduced
// search still has at least one ply.
func (s *SearchState) lmrReduction(pos Position, m Move, depth, i, ply int, pvNode, inCheck bool) int {
	if inCheck || depth < 3 || i < 1 || isCapture(pos, m) || m.IsPromotion() {
		return 0
	}

	r := lmrTable[min(depth, len(lmrTable)-1)][min(i, len(lmrTable[0])-1)]
	if pvNode {
		r--
	}
	if s.isKiller(ply, m) {
		r--
	}
	if s.historyBonus(pos.SideToMove, m) >= lmrHistoryThreshold {
		r--
	}

	return max(0, min(r, depth-2))
}
