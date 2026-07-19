package engine

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
