package engine

const totalPhase = 24

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

var phaseWeights = [7]int{
	Knight: 1,
	Bishop: 1,
	Rook:   2,
	Queen:  4,
}

func Evaluate(pos Position) int {
	var eval int

	phase := gamePhase(pos)

	for pt := Pawn; pt <= King; pt++ {

		whitePieces := pos.Pieces[pt] & pos.Colors[White]
		for whitePieces != 0 {
			sq := whitePieces.PopLSB()

			// Change PST for King in the endgame
			pstValue := pst[pt][sq^56]
			if pt == King {
				pstValue = (pst[King][sq^56]*phase + kingEndgamePST[sq^56]*(totalPhase-phase)) / totalPhase
			}

			eval += pieceValues[pt] + pstValue
		}

		blackPieces := pos.Pieces[pt] & pos.Colors[Black]
		for blackPieces != 0 {
			sq := blackPieces.PopLSB()

			// Change PST for King in the endgame
			pstValue := pst[pt][sq]
			if pt == King {
				pstValue = (pst[King][sq]*phase + kingEndgamePST[sq]*(totalPhase-phase)) / totalPhase
			}

			eval -= pieceValues[pt] + pstValue
		}
	}

	return eval
}

func gamePhase(pos Position) int {
	var phase int

	for pt := Pawn; pt <= King; pt++ {
		pieces := pos.Pieces[pt]
		phase += pieces.PopCount() * phaseWeights[pt]
	}

	return min(phase, totalPhase)
}
