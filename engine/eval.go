package engine

const totalPhase = 24

const (
	knightMobilityBonus = 4
	bishopMobilityBonus = 3
	rookMobilityBonus   = 2
	queenMobilityBonus  = 1
)

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

			// Add PST value for all pieces
			pstValue := pst[pt][sq^56]

			// Change PST for King in the endgame
			if pt == King {
				pstValue = (pst[King][sq^56]*phase + kingEndgamePST[sq^56]*(totalPhase-phase)) / totalPhase
			}

			// Mobility bonus
			mobilityBonus := 0
			switch pt {
			case Knight:
				mobilityBonus = (KnightAttacks[sq] &^ pos.Colors[White]).PopCount() * knightMobilityBonus
			case Bishop:
				mobilityBonus = (bishopAttacksMagic(sq, pos.Pieces[AllPieces]) &^ pos.Colors[White]).PopCount() * bishopMobilityBonus
			case Rook:
				mobilityBonus = (rookAttacksMagic(sq, pos.Pieces[AllPieces]) &^ pos.Colors[White]).PopCount() * rookMobilityBonus
			case Queen:
				mobilityBonus = (queenAttacksMagic(sq, pos.Pieces[AllPieces]) &^ pos.Colors[White]).PopCount() * queenMobilityBonus
			}

			eval += pieceValues[pt] + pstValue + mobilityBonus
		}

		blackPieces := pos.Pieces[pt] & pos.Colors[Black]
		for blackPieces != 0 {
			sq := blackPieces.PopLSB()

			// Add PST value for all pieces
			pstValue := pst[pt][sq]

			// Change PST for King in the endgame
			if pt == King {
				pstValue = (pst[King][sq]*phase + kingEndgamePST[sq]*(totalPhase-phase)) / totalPhase
			}

			// Mobility bonus
			mobilityBonus := 0
			switch pt {
			case Knight:
				mobilityBonus = (KnightAttacks[sq] &^ pos.Colors[Black]).PopCount() * knightMobilityBonus
			case Bishop:
				mobilityBonus = (bishopAttacksMagic(sq, pos.Pieces[AllPieces]) &^ pos.Colors[Black]).PopCount() * bishopMobilityBonus
			case Rook:
				mobilityBonus = (rookAttacksMagic(sq, pos.Pieces[AllPieces]) &^ pos.Colors[Black]).PopCount() * rookMobilityBonus
			case Queen:
				mobilityBonus = (queenAttacksMagic(sq, pos.Pieces[AllPieces]) &^ pos.Colors[Black]).PopCount() * queenMobilityBonus
			}

			eval -= pieceValues[pt] + pstValue + mobilityBonus
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
