package engine

import (
	"context"
	"slices"
	"time"
)

const (
	MateValue = 100000
	Minimum   = -MateValue - 1
	Maximum   = MateValue + 1
)

type SearchState struct {
	ctx        context.Context
	nodes      uint64
	killers    [64][2]Move
	historyHeu [2][64][64]int
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

	// Take last moves from history. Clamped to 0: when search starts from a
	// FEN with a nonzero halfmove clock, history only holds moves made
	// since the search began, which can be fewer than FiftyMovesRule.
	start := max(0, len(history)-1-pos.FiftyMovesRule)
	lastHistory := history[start:]

	// Check for a draw by threefold repetition.
	count := 0
	for _, h := range lastHistory {
		if h == hash {
			count++
		}
	}
	if count >= 3 {
		return 0
	}

	// Check FiftyMovesRule
	if pos.FiftyMovesRule >= 100 {
		return 0
	}

	// Check position in the transpositional table
	entry, found := ttProbe(hash)
	adjustedScore := adjustMateForRetreve(entry.score, ply)
	if found && entry.depth >= depth {
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

	// Static null move pruning
	var staticEval int
	if beta-alpha == 1 && !pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) && beta < MateValue-1000 {
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
	if !nullMove && depth >= 4 && !pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) && hasNonPawnMaterial(*pos, pos.SideToMove) {
		R := 3

		oldEnPassantSquare := pos.MakeNullMove()

		nullScore := -s.Negamax(pos, depth-1-R, ply+1, -beta, -beta+1, history, true, extensions)

		pos.UnmakeNullMove(oldEnPassantSquare)

		if nullScore >= beta {
			return beta
		}
	}

	var moveList MoveList
	GenerateLegalMoves(*pos, pos.SideToMove, &moveList)
	moves := moveList.Slice()

	// Order moves (ttMove => captures => killers => others)
	slices.SortFunc(moves, func(a, b Move) int {
		return s.orderScore(*pos, ply, b, entry.bestMove) - s.orderScore(*pos, ply, a, entry.bestMove)
	})

	// Check Mate/Stalemate
	if len(moves) == 0 && pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
		return -(MateValue - ply)
	} else if len(moves) == 0 && !pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
		return 0
	}

	// Variables for creating ttEntry
	var currentAlpha = alpha
	var bestMove Move
	var flag TTFlag

	for i, move := range moves {
		undo := pos.MakeMove(move)
		newHistory := append(history, pos.Hash()) // Save new position's hash to history

		search := func(a, b int, reduced bool) int {
			// Check extension
			if pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) && extensions < 16 {
				return -s.Negamax(pos, depth, ply+1, a, b, newHistory, false, extensions+1)
			}

			newDepth := depth - 1
			if reduced {
				newDepth--
			}

			return -s.Negamax(pos, newDepth, ply+1, a, b, newHistory, false, extensions)
		}

		// LMR
		canReduce := i > 0 && s.canReduce(*pos, move, depth, i, ply)

		// Principal Variation Search
		var nextEval int
		if i == 0 {
			nextEval = search(-beta, -alpha, false)
		} else {
			nextEval = search(-alpha-1, -alpha, canReduce)

			if canReduce && nextEval > alpha {
				nextEval = search(-alpha-1, -alpha, false)
			}

			if nextEval > alpha && beta-alpha > 1 {
				nextEval = search(-beta, -alpha, false)
			}
		}

		pos.UnmakeMove(move, undo)

		if nextEval >= beta {

			// Store killer moves
			if !isCapture(*pos, move) && (ply >= len(s.killers) || s.killers[ply][0] != move) {
				s.storeKiller(ply, move)

				// History heuristic
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
		GenerateLegalMoves(*pos, pos.SideToMove, &moveList)

		// Check if Mate
		if moveList.count == 0 && pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
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

		GenerateLegalCaptures(*pos, pos.SideToMove, &moveList)
	}

	moves := moveList.Slice()

	// MVV-LVA
	slices.SortFunc(moves, func(a, b Move) int {
		return moveScore(*pos, b) - moveScore(*pos, a)
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

// BestMove returns the best move found by a fixed-depth negamax search.
func BestMove(ctx context.Context, pos Position, depth int, history []uint64, alpha, beta int) (Move, uint64, int) {
	s := &SearchState{ctx: ctx}
	var moveList MoveList
	GenerateLegalMoves(pos, pos.SideToMove, &moveList)
	moves := moveList.Slice()

	if len(moves) == 0 {
		if pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
			return Move(0), 0, -(MateValue + 0)
		}
		return Move(0), 0, 0
	}

	hash := pos.Hash()
	entry, _ := ttProbe(hash)

	// Ordering move ttMove => Captures => Others
	slices.SortFunc(moves, func(a, b Move) int {
		return s.orderScore(pos, 0, b, entry.bestMove) - s.orderScore(pos, 0, a, entry.bestMove)
	})

	bestMove := moves[0]
	bestScore := Minimum

	for _, move := range moves {
		undo := pos.MakeMove(move)

		newHistory := append(history, pos.Hash())

		score := -s.Negamax(&pos, depth-1, 1, -beta, -alpha, newHistory, false, 0)

		pos.UnmakeMove(move, undo)

		if score > bestScore {
			bestScore = score
			bestMove = move
		}
	}

	return bestMove, s.nodes, bestScore
}

// SearchTimed performs iterative deepening negamax search, returning
// the best move found before timeLimit expires along with the total node
// count and the deepest depth fully searched. depth 1 always seeds bestMove
// with a real legal move (even under extreme time pressure), and later
// depths only overwrite it once they finish completely -- a depth cut short
// by the time limit is discarded rather than allowed to replace a good,
// fully-searched result with a worse, half-searched one.
func SearchTimed(pos Position, timeLimit time.Duration, history []uint64) (Move, uint64, int, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeLimit)
	defer cancel()

	var completedDepth int

	bestMove, totalNodes, bestScore := BestMove(ctx, pos, 1, history, Minimum, Maximum)
	if ctx.Err() == nil {
		completedDepth = 1
	}

	windowSize := 50
	alpha, beta := bestScore-windowSize, bestScore+windowSize

	for depth := 2; ; depth++ {
		if ctx.Err() != nil {
			break
		}

		move, nodes, score := BestMove(ctx, pos, depth, history, alpha, beta)
		totalNodes += nodes

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
		completedDepth = depth

		alpha, beta = score-windowSize, score+windowSize
	}

	return bestMove, totalNodes, completedDepth, bestScore
}

// moveScore returns a priority score for move ordering: captures
// of valuable pieces by less valuable attackers score highest (MVV-LVA).
func moveScore(pos Position, m Move) int {
	captured := pos.PieceAt(m.To())

	if captured == AllPieces {
		attacker := pos.PieceAt(m.From())
		if attacker == Pawn && m.From().File() != m.To().File() {
			return pieceValues[Pawn]*10 - pieceValues[Pawn]
		}
		return 0
	}
	attacker := pos.PieceAt(m.From())
	return pieceValues[captured]*10 - pieceValues[attacker]
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

func SearchDepth(pos Position, maxDepth int, history []uint64) (Move, uint64, int, int) {
	ctx := context.Background()

	var completedDepth int
	bestMove, totalNodes, bestScore := BestMove(ctx, pos, 1, history, Minimum, Maximum)

	// Aspiration window
	windowSize := 50
	alpha, beta := bestScore-windowSize, bestScore+windowSize

	for depth := 2; depth <= maxDepth; depth++ {
		move, nodes, score := BestMove(ctx, pos, depth, history, alpha, beta)

		totalNodes += nodes

		if score <= alpha || score >= beta {
			alpha, beta = Minimum, Maximum
			depth--
			continue
		}

		bestMove = move
		bestScore = score
		completedDepth = depth

		alpha, beta = score-windowSize, score+windowSize
	}

	return bestMove, totalNodes, completedDepth, bestScore
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

	score := moveScore(pos, m)

	if ply >= len(s.killers) {
		return score
	}

	if !isCapture(pos, m) {
		switch m {
		case s.killers[ply][0]:
			score += 50
		case s.killers[ply][1]:
			score += 40
		default:
			// History heuristic
			score += min(s.historyHeu[pos.SideToMove][m.From()][m.To()]/1000, 39)
		}
	}

	return score
}

func (s *SearchState) canReduce(pos Position, move Move, depth, i, ply int) bool {
	if ply >= len(s.killers) {
		return depth >= 3 && i >= 4 && !isCapture(pos, move) && !move.IsPromotion()
	}

	if depth >= 3 && i >= 4 && !isCapture(pos, move) && !move.IsPromotion() && move != s.killers[ply][0] && move != s.killers[ply][1] {
		return true
	}

	return false
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
