package engine

import (
	"context"
	"slices"
)

const (
	MateValue = 100000
	Minimum   = -MateValue - 1
	Maximum   = MateValue + 1
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

type SearchState struct {
	ctx        context.Context
	nodes      uint64
	killers    [64][2]Move
	historyHeu [2][64][64]int

	// rootIdx is the index of the root position in the history slice passed to
	// Negamax: entries before it are game history, entries from it on were
	// reached during this search. Set by BestMove.
	rootIdx int

	// rootBest is the best move found by the last completed root search.
	// The next iteration searches it first. Zero before the first iteration.
	rootBest Move
}

// Negamax performs a depth-limited negamax search with alpha-beta pruning
// and returns the score from the perspective of the side to move.
func (s *SearchState) Negamax(pos *Position, depth, ply int, alpha, beta int, history []uint64, nullMove bool, extensions int) int {
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

	// Static null move pruning
	var staticEval int
	if beta-alpha == 1 && !inCheck && beta < MateValue-1000 {
		if pos.SideToMove == White {
			staticEval = Evaluate(pos)
		} else {
			staticEval = -Evaluate(pos)
		}

		margin := 85 * depth
		if staticEval-margin >= beta {
			return beta
		}

	}

	// Null move logic
	if !nullMove && depth >= 4 && !inCheck && hasNonPawnMaterial(*pos, pos.SideToMove) {
		R := 3

		oldEnPassantSquare := pos.MakeNullMove()

		nullScore := -s.Negamax(pos, depth-1-R, ply+1, -beta, -beta+1, history, true, extensions)

		pos.UnmakeNullMove(oldEnPassantSquare)

		if nullScore >= beta {
			return beta
		}
	}

	var moveList MoveList
	generateLegalMoves(*pos, pos.SideToMove, inCheck, &moveList)
	moves := moveList.Slice()

	// Order moves (ttMove => captures => killers => others)
	sortMovesByScore(moves, func(m Move) int {
		return s.orderScore(*pos, ply, m, entry.bestMove)
	})

	// Check Mate/Stalemate
	if len(moves) == 0 {
		if inCheck {
			return -(MateValue - ply)
		}
		return 0
	}

	// Variables for creating ttEntry
	var currentAlpha = alpha
	var bestMove Move
	var flag TTFlag

	for i, move := range moves {
		// LMR: decided before the move is made, since lmrReduction looks at what
		// the move captures and after MakeMove the target square holds the mover.
		reduction := s.lmrReduction(*pos, move, depth, i, ply, beta-alpha > 1, inCheck)

		undo := pos.MakeMove(move)
		newHistory := append(history, pos.Hash()) // Save new position's hash to history

		search := func(a, b, reduction int) int {
			// Check extension
			if pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) && extensions < 16 {
				return -s.Negamax(pos, depth, ply+1, a, b, newHistory, false, extensions+1)
			}

			return -s.Negamax(pos, depth-1-reduction, ply+1, a, b, newHistory, false, extensions)
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

	// Ordering move ttMove => Captures => Others
	sortMovesByScore(moves, func(m Move) int {
		return s.orderScore(pos, 0, m, ttMove)
	})

	bestMove := moves[0]

	for i, move := range moves {
		undo := pos.MakeMove(move)
		newHistory := append(history, pos.Hash())

		var score int
		if i == 0 {
			score = -s.Negamax(&pos, depth-1, 1, -beta, -alpha, newHistory, false, 0)
		} else {
			// Null window: we only need to know whether this move beats alpha.
			score = -s.Negamax(&pos, depth-1, 1, -alpha-1, -alpha, newHistory, false, 0)
			if score > alpha && score < beta {
				score = -s.Negamax(&pos, depth-1, 1, -beta, -alpha, newHistory, false, 0)
			}
		}

		pos.UnmakeMove(move, undo)

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
// would then rank below the killers (50/40) and the quiet moves; adding the
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
	// are still searched late.
	var promotionBonus int
	if isQueenPromotion(m) {
		promotionBonus = (pieceValues[Queen]-pieceValues[Pawn])*10 + noisyBase()
	}

	captured := pos.PieceAt(m.To())

	if captured == AllPieces {
		attacker := pos.PieceAt(m.From())
		if attacker == Pawn && m.From().File() != m.To().File() {
			return pieceValues[Pawn]*10 - pieceValues[Pawn] + noisyBase(), true
		}
		return promotionBonus, false
	}
	attacker := pos.PieceAt(m.From())
	return pieceValues[captured]*10 - pieceValues[attacker] + noisyBase() + promotionBonus, true
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

func (s *SearchState) orderScore(pos Position, ply int, m Move, ttMove Move) int {
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
			score += 50
		case s.killers[ply][1]:
			score += 40
		default:
			// History heuristic, capped below the second killer (40) so it can
			// never outrank a killer. The divisor was picked by node counts over
			// random positions: with 1000 only the top few percent of moves reached
			// a nonzero score, and 30-100 all searched noticeably fewer nodes.
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
