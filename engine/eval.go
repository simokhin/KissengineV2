package engine

var pieceValues = [7]int{
	Pawn:   100,
	Knight: 320,
	Bishop: 330,
	Rook:   500,
	Queen:  900,
	King:   0,
}

// pst holds piece-square tables indexed by piece type, giving each square
// a positional bonus/penalty from White's perspective (mirror via sq^56 for Black)
var pst = [7][64]int{
	Pawn:   pawnPST,
	Knight: knightPST,
	Bishop: bishopPST,
	Rook:   rookPST,
	Queen:  queenPST,
	King:   kingPST,
}

func Evaluate(pos Position) int {
	var eval int

	for pt := Pawn; pt <= King; pt++ {
		whitePieces := pos.Pieces[pt] & pos.Colors[White]
		for whitePieces != 0 {
			sq := whitePieces.PopLSB()
			eval += pieceValues[pt] + pst[pt][sq^56]
		}

		blackPieces := pos.Pieces[pt] & pos.Colors[Black]
		for blackPieces != 0 {
			sq := blackPieces.PopLSB()
			eval -= pieceValues[pt] + pst[pt][sq]
		}
	}

	return eval
}
