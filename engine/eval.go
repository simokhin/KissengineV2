package engine

var pieceValues = [7]int{
	Pawn:   100,
	Knight: 320,
	Bishop: 330,
	Rook:   500,
	Queen:  900,
	King:   0,
}

func Evaluate(pos Position) int {
	var eval int

	for pt := Pawn; pt <= Queen; pt++ {
		whiteCount := (pos.Pieces[pt] & pos.Colors[White]).PopCount()
		blackCount := (pos.Pieces[pt] & pos.Colors[Black]).PopCount()
		eval += (whiteCount - blackCount) * pieceValues[pt]
	}

	return eval
}
