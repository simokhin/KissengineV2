package engine

import (
	"context"
	"time"
)

const (
	MateValue = 100000
	Minimum   = -MateValue - 1
	Maximum   = MateValue + 1
)

// Negamax performs a depth-limited negamax search with alpha-beta pruning
// and returns the score from the perspective of the side to move.
func Negamax(ctx context.Context, pos Position, depth int, nodes *uint64, alpha, beta int) int {
	*nodes++

	select {
	case <-ctx.Done():
		return 0
	default:
	}

	if depth == 0 {
		if pos.SideToMove == White {
			return Evaluate(pos)
		} else {
			return -Evaluate(pos)
		}
	}

	moves := GenerateLegalMoves(pos, pos.SideToMove)
	if len(moves) == 0 && pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
		return -(MateValue + depth)
	} else if len(moves) == 0 && !pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
		return 0
	}

	for _, move := range moves {
		newPos := pos
		newPos.MakeMove(move)
		nextEval := -Negamax(ctx, newPos, depth-1, nodes, -beta, -alpha)
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
func BestMove(ctx context.Context, pos Position, depth int) (Move, uint64) {
	var nodes uint64
	moves := GenerateLegalMoves(pos, pos.SideToMove)

	var bestMove Move
	var bestScore = Minimum

	alpha := Minimum
	beta := Maximum

	for _, move := range moves {
		newPos := pos
		newPos.MakeMove(move)
		score := -Negamax(ctx, newPos, depth-1, &nodes, -beta, -alpha)
		if score > bestScore {
			bestScore = score
			bestMove = move
		}
	}

	return bestMove, nodes
}

func SearchTimed(pos Position, timeLimit time.Duration) (Move, uint64, int) {
	ctx, cancel := context.WithTimeout(context.Background(), timeLimit)
	defer cancel()

	var completedDepth int
	var bestMove Move
	var totalNodes uint64

	for depth := 1; ; depth++ {
		move, nodes := BestMove(ctx, pos, depth)
		totalNodes += nodes
		if ctx.Err() != nil {
			break
		} else {
			bestMove = move
			completedDepth = depth
		}
	}

	return bestMove, totalNodes, completedDepth
}
