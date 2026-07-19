package engine

// perft counts the number of leaf positions reachable from pos after
// depth half-moves, by recursively generating and playing every pseudo-legal move.
func perft(pos Position, depth int) uint64 {
	var nodes uint64

	if depth == 0 {
		return 1
	}

	moves := GenerateMoves(pos, pos.SideToMove)

	for _, move := range moves {
		newPos := pos
		newPos.MakeMove(move)
		nodes += perft(newPos, depth-1)
	}

	return nodes
}
