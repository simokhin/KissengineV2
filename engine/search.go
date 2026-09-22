package engine

import (
	"context"
	"slices"
	"sync/atomic"
)

const (
	MateValue = 100000
	Minimum   = -MateValue - 1
	Maximum   = MateValue + 1
)

// mateBound is the size of a score from which it counts as a mate score (they
// are MateValue minus the ply, so anything closer to MateValue than this).
const mateBound = MateValue - 1000

// lmpLimit[depth] is the index in the move ordering from which a quiet move is
// pruned without being searched, at a node of that depth (late move pruning);
// depths beyond the table are not pruned. The index counts every move before it,
// captures included, and the values (8, 12, 16, 20, 24) are the ones Blunder
// uses; they are untuned here.
var lmpLimit = [...]int{0, 8, 12, 16, 20, 24}

// Futility pruning: at depth <= futilityMaxDepth a node counts as futile when
// staticEval + futilityBase + futilityStep*depth <= alpha. Blunder's values,
// untuned.
const (
	futilityMaxDepth = 6
	futilityBase     = 40
	futilityStep     = 60
)

// losingCaptureOffset is subtracted from the MVV-LVA score of a capture that
// loses material by SEE. It must exceed the largest MVV-LVA score (~23000 with
// the tuned piece values) so every losing capture ranks below every quiet move
// (score >= 0), while staying far below the TT move's 1_000_000.
const losingCaptureOffset = 100_000

// scoredMove pairs a move with its precomputed ordering score.
type scoredMove struct {
	move  Move
	score int
}

// sortMovesByScore reorders moves in place by descending score(m). Computes
// each move's score exactly once (an O(n) pass) instead of the O(n log n)
// score recomputation a comparator calling score(a)/score(b) on every
// slices.SortFunc comparison would do.
func sortMovesByScore(moves []Move, score func(Move) int) {
	var scored [maxMoves]scoredMove
	for i, m := range moves {
		scored[i] = scoredMove{move: m, score: score(m)}
	}

	sorted := scored[:len(moves)]
	slices.SortFunc(sorted, func(a, b scoredMove) int {
		return b.score - a.score
	})

	for i, sm := range sorted {
		moves[i] = sm.move
	}
}

// pickBest moves the highest-scoring move among moves[i:] to position i
// (swapping the scores along) and returns it. Called with i = 0, 1, 2, ... it
// yields the moves in descending score order without sorting the whole list.
func pickBest(moves []Move, scores []int32, i int) Move {
	best := i
	for j := i + 1; j < len(moves); j++ {
		if scores[j] > scores[best] {
			best = j
		}
	}
	moves[i], moves[best] = moves[best], moves[i]
	scores[i], scores[best] = scores[best], scores[i]
	return moves[i]
}

type SearchState struct {
	ctx        context.Context
	nodes      uint64
	killers    [64][2]Move
	historyHeu [2][64][64]int

	// counterMove[side][from][to] is the quiet move by side that last caused a
	// beta cutoff in reply to the opponent's move from->to. Indexed by from/to
	// rather than by ply, unlike killers, so it carries information between
	// unrelated branches of the tree that just happen to answer the same
	// threat, not only within one line.
	counterMove [2][64][64]Move

	// rootIdx is the index of the root position in the history slice passed to
	// Negamax: entries before it are game history, entries from it on were
	// reached during this search. Set by BestMove.
	rootIdx int

	// rootBest is the best move found by the last completed root search.
	// The next iteration searches it first. Zero before the first iteration.
	rootBest Move

	// unstable is set while the root looks unsettled (the last iteration changed
	// the best move or lowered the score, or this one already has: the first move
	// has fallen or another has taken over). The soft time limit doesn't stop a
	// search while it is set, see SearchSoft. Read by the timer goroutine.
	unstable atomic.Bool

	// prevScore is the score of the last completed iteration, valid if havePrev.
	prevScore int
	havePrev  bool

	// rootPartial is, during a root search, the move that has beaten every move
	// tried before it at the current depth, judged by searches that ran to
	// completion. If the search is cut off, this is the best move known so far
	// (zero if none is: even the first move hasn't finished, or it failed low).
	// Reset by BestMove.
	rootPartial Move
}

// Negamax performs a depth-limited negamax search with alpha-beta pruning
// and returns the score from the perspective of the side to move. prevMove is
// the move that led to this position (Move(0), never a legal move, if there
// isn't one: at the root, or for the position right after a null move), used
// to look up and store the countermove heuristic.
func (s *SearchState) Negamax(pos *Position, depth, ply int, alpha, beta int, history []uint64, nullMove bool, extensions int, prevMove Move) int {
	s.nodes++

	select {
	case <-s.ctx.Done():
		return 0
	default:
	}

	hash := pos.Hash()

	// Repetition, looking only at positions since the last irreversible move.
	// The start is clamped to 0: when the search starts from a FEN with a
	// nonzero halfmove clock, history holds fewer entries than FiftyMovesRule.
	// A node right after a null move isn't a real position, so it can't repeat.
	repeated := false
	if !nullMove {
		n := len(history) - 1 // index of this node's own hash
		for i := max(0, n-pos.FiftyMovesRule); i < n; i++ {
			if history[i] != hash {
				continue
			}

			// An earlier occurrence inside this search (the root included) is
			// scored as a draw: either side can go round the cycle again, so
			// there is nothing to gain from it. Occurrences from before the
			// root only make a draw on the third one, as in the real rule.
			if i >= s.rootIdx || repeated {
				return 0
			}
			repeated = true
		}
	}

	// Check FiftyMovesRule
	if pos.FiftyMovesRule >= 100 {
		return 0
	}

	// Check position in the transpositional table. No cutoff for a position
	// that already occurred in the game: the stored score knows nothing about
	// the repetition history, so it can hide that the opponent now completes a
	// threefold. The TT move is still used for ordering below.
	entry, found := ttProbe(hash)
	adjustedScore := adjustMateForRetreve(entry.score, ply)
	if found && entry.depth >= depth && !repeated {
		switch entry.flag {
		case Exact:
			return adjustedScore
		case LowerBound:
			if adjustedScore >= beta {
				return beta
			}
		case UpperBound:
			if adjustedScore <= alpha {
				return alpha
			}
		}
	}

	if depth == 0 {
		return Quiescence(s.ctx, pos, &s.nodes, alpha, beta, ply)
	}

	inCheck := pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1)

	// The static evaluation, computed at null-window nodes outside check (the
	// only ones static null move and futility pruning look at); zero otherwise.
	var staticEval int
	if beta-alpha == 1 && !inCheck {
		if pos.SideToMove == White {
			staticEval = Evaluate(pos)
		} else {
			staticEval = -Evaluate(pos)
		}

		// Static null move pruning
		margin := 85 * depth
		if beta < mateBound && staticEval-margin >= beta {
			return beta
		}
	}

	// Null move logic. Skipped when beta is a mate score, same as static null
	// move above: a fail-high there would rest on an unproven mate found by a
	// reduced-depth search, which can mask a real forced mate or plant a
	// spurious mate-bound score in the TT.
	if !nullMove && depth >= 4 && !inCheck && beta < mateBound && hasNonPawnMaterial(*pos, pos.SideToMove) {
		// R grows with depth so the reduction stays proportionally cheap at
		// high depth instead of eating a shrinking fraction of it.
		R := 3 + depth/6

		// If an Exact/UpperBound entry already covers at least as much depth as
		// the reduced null-move search would and its score is below beta, that
		// search would very likely fail low too (a LowerBound entry says
		// nothing here: it only proves a floor, not that beta is out of
		// reach). Skip it and save those nodes.
		skip := found && entry.depth >= depth-1-R && entry.flag != LowerBound && adjustedScore < beta

		if !skip {
			oldEnPassantSquare := pos.MakeNullMove()

			nullScore := -s.Negamax(pos, depth-1-R, ply+1, -beta, -beta+1, history, true, extensions, Move(0))

			pos.UnmakeNullMove(oldEnPassantSquare)

			if nullScore >= beta {
				return beta
			}
		}
	}

	var moveList MoveList
	generateLegalMoves(*pos, pos.SideToMove, inCheck, &moveList)
	moves := moveList.Slice()

	// Check Mate/Stalemate
	if len(moves) == 0 {
		if inCheck {
			return -(MateValue - ply)
		}
		return 0
	}

	// The countermove heuristic's suggestion for this node: the quiet move
	// that last refuted prevMove elsewhere in the tree. Move(0) (no
	// suggestion) both when there is no prevMove and when none was ever
	// stored for it, since the table starts zero-valued.
	var counterMove Move
	if prevMove != 0 {
		counterMove = s.counterMove[pos.SideToMove][prevMove.From()][prevMove.To()]
	}

	// Order moves (ttMove => captures => killers => countermove => others):
	// score once here, then pickBest selects the next move lazily, so a node
	// that cuts off on its first moves never orders the rest.
	var scores [maxMoves]int32
	for i, m := range moves {
		scores[i] = int32(s.orderScore(*pos, ply, m, entry.bestMove, counterMove))
	}

	// Variables for creating ttEntry
	var currentAlpha = alpha
	var bestMove Move
	var flag TTFlag

	pvNode := beta-alpha > 1

	// Late move and futility pruning apply to null-window nodes outside check
	// (where staticEval was computed), and not while alpha is a mate score:
	// skipping quiet moves there could miss the only defence and report a mate
	// that isn't forced.
	canPrune := !pvNode && !inCheck && alpha > -mateBound && alpha < mateBound

	// Futility: near the horizon, if even a generous margin on top of the static
	// evaluation stays at or below alpha, quiet moves after the first are not
	// expected to raise it and are skipped.
	futile := canPrune && depth <= futilityMaxDepth &&
		staticEval+futilityBase+futilityStep*depth <= alpha

	for i := range moves {
		move := pickBest(moves, scores[:len(moves)], i)

		// Both need the position before the move: after MakeMove the target
		// square holds the mover, so a capture no longer looks like one.
		quiet := !isCapture(*pos, move) && !move.IsPromotion()
		reduction := s.lmrReduction(*pos, move, depth, i, ply, pvNode, inCheck)

		undo := pos.MakeMove(move)
		givesCheck := pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1)

		// Late move pruning: near the horizon, a quiet move that the ordering put
		// this far back (and that doesn't check) is not searched at all. The
		// same goes for any quiet move after the first at a futile node.
		if canPrune && quiet && !givesCheck &&
			((futile && i > 0) || (depth < len(lmpLimit) && i >= lmpLimit[depth])) {
			pos.UnmakeMove(move, undo)
			continue
		}

		newHistory := append(history, pos.Hash()) // Save new position's hash to history

		search := func(a, b, reduction int) int {
			// Check extension
			if givesCheck && extensions < 16 {
				return -s.Negamax(pos, depth, ply+1, a, b, newHistory, false, extensions+1, move)
			}

			return -s.Negamax(pos, depth-1-reduction, ply+1, a, b, newHistory, false, extensions, move)
		}

		// Principal Variation Search
		var nextEval int
		if i == 0 {
			nextEval = search(-beta, -alpha, 0)
		} else {
			nextEval = search(-alpha-1, -alpha, reduction)

			if reduction > 0 && nextEval > alpha {
				nextEval = search(-alpha-1, -alpha, 0)
			}

			if nextEval > alpha && beta-alpha > 1 {
				nextEval = search(-beta, -alpha, 0)
			}
		}

		pos.UnmakeMove(move, undo)

		if nextEval >= beta {

			if !isCapture(*pos, move) {
				// Store killer moves
				if ply >= len(s.killers) || s.killers[ply][0] != move {
					s.storeKiller(ply, move)
				}

				// History heuristic: credited even when the move is already the
				// first killer, so moves that keep causing cutoffs keep gaining.
				s.historyHeu[pos.SideToMove][move.From()][move.To()] += depth * depth

				// Countermove heuristic: remember move as the reply to prevMove,
				// so a sibling node elsewhere in the tree that faces the same
				// threat tries it early too.
				if prevMove != 0 {
					s.counterMove[pos.SideToMove][prevMove.From()][prevMove.To()] = move
				}
			}

			// Store position in tTable
			flag = LowerBound
			if s.ctx.Err() == nil {
				ttStore(hash, depth, adjustMateForStore(beta, ply), LowerBound, move)
			}

			return beta
		}
		if nextEval > alpha {
			bestMove = move
			alpha = nextEval
		}
	}

	if alpha > currentAlpha {
		flag = Exact
	} else if alpha == currentAlpha {
		flag = UpperBound
	}

	// Store position in tTable
	if s.ctx.Err() == nil {
		ttStore(hash, depth, adjustMateForStore(alpha, ply), flag, bestMove)
	}

	return alpha
}

// Quiescence extends the search beyond the normal depth limit by only considering captures
// (or, if in check, all legal moves), continuing until the position becomes "quiet".
// This avoids the horizon effect, where a fixed-depth search stops mid-exchange and
// misjudges the position.
func Quiescence(ctx context.Context, pos *Position, nodes *uint64, alpha, beta, ply int) int {
	*nodes++

	// Check if we have time
	select {
	case <-ctx.Done():
		return 0
	default:
	}

	var moveList MoveList

	var standPat int

	isCheck := pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1)
	if isCheck {
		// Generate moves as usual
		generateLegalMoves(*pos, pos.SideToMove, isCheck, &moveList)

		// Check if Mate
		if moveList.count == 0 {
			return -(MateValue - ply)
		}

	} else {
		if pos.SideToMove == White {
			standPat = Evaluate(pos)
		} else {
			standPat = -Evaluate(pos)
		}

		if standPat >= beta {
			return beta
		} else if standPat > alpha {
			alpha = standPat
		}

		GenerateLegalNoisyMoves(*pos, pos.SideToMove, &moveList)
	}

	moves := moveList.Slice()

	// MVV-LVA
	sortMovesByScore(moves, func(m Move) int {
		score, _ := moveScore(*pos, m)
		return score
	})

	for _, move := range moves {
		undo := pos.MakeMove(move)

		nextEval := -Quiescence(ctx, pos, nodes, -beta, -alpha, ply+1)

		pos.UnmakeMove(move, undo)

		if nextEval >= beta {
			return beta
		}
		if nextEval > alpha {
			alpha = nextEval
		}
	}

	return alpha
}

// BestMove returns the best move found by a fixed-depth negamax search, along
// with the number of nodes this call searched. s carries the killer/history
// tables, so callers running iterative deepening pass the same s to every
// iteration and later depths reuse what earlier ones learned about good quiet
// moves; s.nodes is reset here, so the returned count is per call.
func BestMove(s *SearchState, pos Position, depth int, history []uint64, alpha, beta int) (Move, uint64, int) {
	s.nodes = 0
	s.rootIdx = len(history) - 1
	var moveList MoveList
	GenerateLegalMoves(pos, pos.SideToMove, &moveList)
	moves := moveList.Slice()

	if len(moves) == 0 {
		if pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
			return Move(0), 0, -(MateValue + 0)
		}
		return Move(0), 0, 0
	}

	// Previous iteration's best move first; the TT is only a fallback for the
	// very first iteration, where it can hold a move from an earlier `go`.
	ttMove := s.rootBest
	if ttMove == 0 {
		entry, _ := ttProbe(pos.Hash())
		ttMove = entry.bestMove
	}

	// Ordering move ttMove => Captures => Others. No countermove suggestion
	// here: the root has no prevMove tracked as a Move (only the game history
	// of hashes), and it already orders well from ttMove/rootBest.
	sortMovesByScore(moves, func(m Move) int {
		return s.orderScore(pos, 0, m, ttMove, Move(0))
	})

	bestMove := moves[0]
	s.rootPartial = 0

	for i, move := range moves {
		undo := pos.MakeMove(move)
		newHistory := append(history, pos.Hash())

		// A search that was cut off returns garbage, so a score only counts (for
		// bestMove and for rootPartial) if the context was still alive after it.
		var score int
		if i == 0 {
			score = -s.Negamax(&pos, depth-1, 1, -beta, -alpha, newHistory, false, 0, move)
			if s.ctx.Err() == nil && score > alpha {
				s.rootPartial = move
			}
			if s.ctx.Err() == nil && s.havePrev && score <= s.prevScore-unstableDrop {
				s.unstable.Store(true) // the move that looked best has fallen
			}
		} else {
			// Null window: we only need to know whether this move beats alpha.
			score = -s.Negamax(&pos, depth-1, 1, -alpha-1, -alpha, newHistory, false, 0, move)
			if s.ctx.Err() == nil && score > alpha && score < beta {
				// It beats the best move so far, however the re-search below ends.
				previous := s.rootPartial
				s.rootPartial = move
				s.unstable.Store(true) // another move has taken over
				score = -s.Negamax(&pos, depth-1, 1, -beta, -alpha, newHistory, false, 0, move)
				if s.ctx.Err() == nil && score <= alpha {
					s.rootPartial = previous // the re-search didn't confirm it
				}
			} else if s.ctx.Err() == nil && score > alpha {
				s.rootPartial = move // fail-high: at least beta
				s.unstable.Store(true)
			}
		}

		pos.UnmakeMove(move, undo)

		if s.ctx.Err() != nil {
			break
		}

		if score > alpha {
			alpha = score
			bestMove = move
			if alpha >= beta {
				break // fail-high: the caller widens the aspiration window
			}
		}
	}

	// A cancelled search returns garbage, so it must not become the next
	// iteration's first move.
	if s.ctx.Err() == nil {
		s.rootBest = bestMove
	}

	return bestMove, s.nodes, alpha
}

// noisyBase is added to the score of every capture and queen promotion in
// moveScore. The raw MVV-LVA value, victim*10 - attacker, is negative for a
// queen taking a pawn once the queen is worth more than ten pawns, and it
// would then rank below the killers (60/50) and the quiet moves; adding the
// queen's value (the largest attacker) keeps every such move at or above
// 10*pawn, whatever pieceValues holds after a retune. It shifts all these moves
// alike, so their order among themselves is the plain MVV-LVA order.
// TestNoisyScoreBands checks the resulting bands.
func noisyBase() int {
	return pieceValues[Queen]
}

// moveScore returns a priority score for move ordering: captures
// of valuable pieces by less valuable attackers score highest (MVV-LVA).
// The second return value reports whether m is a capture (including en
// passant), so callers that need both don't also have to call isCapture
// and redo the same PieceAt lookups.
func moveScore(pos Position, m Move) (score int, capture bool) {
	// A queen promotion gains about a queen minus the pawn, on the same x10
	// scale as the capture values below; underpromotions get no bonus so they
	// are still searched late. This is a pure bonus, not itself shifted by
	// noisyBase: a capturing promotion must only get that shift once, from
	// the capture below, not once per noisy component of the move.
	var promotionGain int
	if isQueenPromotion(m) {
		promotionGain = (pieceValues[Queen] - pieceValues[Pawn]) * 10
	}

	captured := pos.PieceAt(m.To())

	if captured == AllPieces {
		attacker := pos.PieceAt(m.From())
		if attacker == Pawn && m.From().File() != m.To().File() {
			return pieceValues[Pawn]*10 - pieceValues[Pawn] + noisyBase(), true
		}
		if promotionGain == 0 {
			return 0, false
		}
		return promotionGain + noisyBase(), false
	}
	attacker := pos.PieceAt(m.From())
	return pieceValues[captured]*10 - pieceValues[attacker] + promotionGain + noisyBase(), true
}

// isQueenPromotion reports whether move m promotes a pawn to a queen.
func isQueenPromotion(m Move) bool {
	return m.IsPromotion() && m.Promotion() == Queen
}

// isCapture reports whether move m is a capture
// (including en passant)
func isCapture(pos Position, m Move) bool {
	if pos.PieceAt(m.To()) != AllPieces {
		return true
	} else {
		// En passant capture check
		if pos.PieceAt(m.From()) == Pawn && m.From().File() != m.To().File() {
			return true
		}
		return false
	}
}

func (p *Position) MakeNullMove() Square {
	p.hash ^= zobristSideToMove
	p.SideToMove ^= 1

	oldEnPassant := p.EnPassant
	if p.EnPassant != NoSquare {
		p.hash ^= zobristEnPassantFile[p.EnPassant.File()]
	}
	p.EnPassant = NoSquare

	return oldEnPassant
}

func (p *Position) UnmakeNullMove(oldEnPassant Square) {
	p.hash ^= zobristSideToMove
	p.SideToMove ^= 1

	if oldEnPassant != NoSquare {
		p.hash ^= zobristEnPassantFile[oldEnPassant.File()]
	}
	p.EnPassant = oldEnPassant
}

func hasNonPawnMaterial(pos Position, color Color) bool {
	return (pos.Pieces[Knight]|pos.Pieces[Bishop]|pos.Pieces[Rook]|pos.Pieces[Queen])&pos.Colors[color] != 0
}

func (s *SearchState) storeKiller(ply int, move Move) {
	if ply >= len(s.killers) {
		return
	}

	s.killers[ply][1] = s.killers[ply][0]
	s.killers[ply][0] = move
}

func (s *SearchState) orderScore(pos Position, ply int, m Move, ttMove Move, counterMove Move) int {
	if m == ttMove {
		return 1_000_000
	}

	score, capture := moveScore(pos, m)

	// A capture that loses material in the exchange on its square is tried
	// after every quiet move, killers and history moves included.
	if capture && isLosingCapture(&pos, m) {
		score -= losingCaptureOffset
	}

	if ply >= len(s.killers) {
		return score
	}

	if !capture {
		switch m {
		case s.killers[ply][0]:
			score += 60
		case s.killers[ply][1]:
			score += 50
		case counterMove:
			// counterMove is Move(0) (never a legal move) when there was no
			// prevMove or nothing is stored for it yet, so this case can only
			// match a real move.
			score += 40
		default:
			// History heuristic, capped below the countermove bonus (40) so it
			// can never outrank a killer or the countermove. The divisor was
			// picked by node counts over random positions: with 1000 only the
			// top few percent of moves reached a nonzero score, and 30-100 all
			// searched noticeably fewer nodes.
			score += s.historyBonus(pos.SideToMove, m)
		}
	}

	return score
}

func adjustMateForStore(score, ply int) int {
	if score >= MateValue-1000 {
		return score + ply
	}
	if score <= -(MateValue - 1000) {
		return score - ply
	}
	return score
}

func adjustMateForRetreve(score, ply int) int {
	if score >= MateValue-1000 {
		return score - ply
	}
	if score <= -(MateValue - 1000) {
		return score + ply
	}
	return score
}
