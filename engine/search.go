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

// Negamax performs a depth-limited negamax search with alpha-beta pruning
// and returns the score from the perspective of the side to move.
func Negamax(ctx context.Context, pos Position, depth int, nodes *uint64, alpha, beta int, history []uint64) int {
	*nodes++

	select {
	case <-ctx.Done():
		return 0
	default:
	}

	// Check for a draw by threefold repetition.
	count := 0
	for _, h := range history {
		if h == pos.Hash() {
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
	hash := pos.Hash()
	entry, found := ttProbe(hash)
	if found && entry.depth >= depth {
		switch entry.flag {
		case Exact:
			return entry.score
		case LowerBound:
			if entry.score >= beta {
				return beta
			}
		case UpperBound:
			if entry.score <= alpha {
				return alpha
			}
		}
	}

	if depth == 0 {
		return Quiescence(ctx, pos, nodes, alpha, beta)
	}

	moves := GenerateLegalMoves(pos, pos.SideToMove)

	// MVV-LVA
	slices.SortFunc(moves, func(a, b Move) int {
		return moveScore(pos, b) - moveScore(pos, a)
	})

	// Check Mate/Stalemate
	if len(moves) == 0 && pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
		return -(MateValue + depth)
	} else if len(moves) == 0 && !pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
		return 0
	}

	// Variables for creating ttEntry
	var currentAlpha = alpha
	var bestMove Move
	var flag TTFlag

	for _, move := range moves {
		newPos := pos
		newPos.MakeMove(move)

		newHistory := append(history, newPos.Hash()) // Save new position's hash to history

		nextEval := -Negamax(ctx, newPos, depth-1, nodes, -beta, -alpha, newHistory)
		if nextEval >= beta {

			// Store position in tTable
			flag = LowerBound
			ttStore(hash, depth, beta, LowerBound, move)

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
	ttStore(hash, depth, alpha, flag, bestMove)

	return alpha
}

// Quiescence extends the search beyond the normal depth limit by only considering captures
// (or, if in check, all legal moves), continuing until the position becomes "quiet".
// This avoids the horizon effect, where a fixed-depth search stops mid-exchange and
// misjudges the position.
func Quiescence(ctx context.Context, pos Position, nodes *uint64, alpha, beta int) int {
	*nodes++

	// Check if we have time
	select {
	case <-ctx.Done():
		return 0
	default:
	}

	var moves []Move

	var standPat int

	isCheck := pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1)
	if isCheck {
		// Generate moves as usual
		moves = GenerateLegalMoves(pos, pos.SideToMove)

		// Check if Mate
		if len(moves) == 0 && pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
			return -(MateValue + 1)
		}

	} else {
		unsortedMoves := GenerateLegalMoves(pos, pos.SideToMove)

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

		// Take only capture moves
		for _, move := range unsortedMoves {
			if isCapture(pos, move) {
				moves = append(moves, move)
			} else {
				continue
			}
		}
	}

	// MVV-LVA
	slices.SortFunc(moves, func(a, b Move) int {
		return moveScore(pos, b) - moveScore(pos, a)
	})

	for _, move := range moves {
		newPos := pos
		newPos.MakeMove(move)

		nextEval := -Quiescence(ctx, newPos, nodes, -beta, -alpha)
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
func BestMove(ctx context.Context, pos Position, depth int, history []uint64) (Move, uint64, int) {
	var nodes uint64
	moves := GenerateLegalMoves(pos, pos.SideToMove)

	// MVV-LVA
	slices.SortFunc(moves, func(a, b Move) int {
		return moveScore(pos, b) - moveScore(pos, a)
	})

	bestMove := moves[0]
	bestScore := Minimum

	alpha := Minimum
	beta := Maximum

	for _, move := range moves {
		newPos := pos
		newPos.MakeMove(move)
		score := -Negamax(ctx, newPos, depth-1, &nodes, -beta, -alpha, history)
		if score > bestScore {
			bestScore = score
			bestMove = move
		}
	}

	return bestMove, nodes, bestScore
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

	bestMove, totalNodes, bestScore := BestMove(ctx, pos, 1, history)
	if ctx.Err() == nil {
		completedDepth = 1
	}

	for depth := 2; ; depth++ {
		if ctx.Err() != nil {
			break
		}

		move, nodes, score := BestMove(ctx, pos, depth, history)
		totalNodes += nodes

		if ctx.Err() != nil {
			break
		}

		bestMove = move
		bestScore = score
		completedDepth = depth
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
	bestMove, totalNodes, bestScore := BestMove(ctx, pos, 1, history)

	for depth := 2; depth <= maxDepth; depth++ {
		move, nodes, score := BestMove(ctx, pos, depth, history)
		totalNodes += nodes
		bestMove = move
		bestScore = score
		completedDepth = depth
	}

	return bestMove, totalNodes, completedDepth, bestScore
}
