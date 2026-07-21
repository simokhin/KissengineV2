package engine

const totalPhase = 24

const (
	knightMobilityBonus = 4
	bishopMobilityBonus = 3
	rookMobilityBonus   = 2
	queenMobilityBonus  = 1

	passedPawnBonus = 20
)

const (
	doubledPawnPenalty  = -10
	isolatedPawnPenalty = -15
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

			// Passed pawn bonus
			var passedBonus int
			if pt == Pawn {
				frontFiles := fileMask(Square(sq.File()))
				if sq.File() > 0 {
					frontFiles |= fileMask(Square(sq.File() - 1))
				}
				if sq.File() < 7 {
					frontFiles |= fileMask(Square(sq.File() + 1))
				}

				var frontRanks Bitboard
				for r := sq.Rank() + 1; r <= 7; r++ {
					frontRanks |= rankMask(Square(r * 8))
				}

				if pos.Pieces[Pawn]&pos.Colors[Black]&frontFiles&frontRanks == 0 {
					passedBonus = passedPawnBonus
				}
			}

			eval += pieceValues[pt] + pstValue + mobilityBonus + passedBonus
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

			// Passed pawn bonus
			var passedBonus int
			if pt == Pawn {
				frontFiles := fileMask(Square(sq.File()))
				if sq.File() > 0 {
					frontFiles |= fileMask(Square(sq.File() - 1))
				}
				if sq.File() < 7 {
					frontFiles |= fileMask(Square(sq.File() + 1))
				}

				var frontRanks Bitboard
				for r := sq.Rank() - 1; r >= 0; r-- {
					frontRanks |= rankMask(Square(r * 8))
				}

				if pos.Pieces[Pawn]&pos.Colors[White]&frontFiles&frontRanks == 0 {
					passedBonus = passedPawnBonus
				}
			}

			eval -= pieceValues[pt] + pstValue + mobilityBonus + passedBonus
		}
	}

	// Doubled/isolated pawn penalty
	var dpPenalty int
	for file := A1; file <= H1; file++ {

		// Check if pawn is doubled
		whiteCount := (pos.Pieces[Pawn] & pos.Colors[White] & fileMask(file)).PopCount()
		if whiteCount > 1 {
			dpPenalty += (whiteCount - 1) * doubledPawnPenalty
		}

		blackCount := (pos.Pieces[Pawn] & pos.Colors[Black] & fileMask(file)).PopCount()
		if blackCount > 1 {
			dpPenalty -= (blackCount - 1) * doubledPawnPenalty
		}

		// Check if pawn is isolated
		whiteLeftEmpty := file == A1 || (pos.Pieces[Pawn]&pos.Colors[White]&fileMask(file-1)).PopCount() == 0
		whiteRightEmpty := file == H1 || (pos.Pieces[Pawn]&pos.Colors[White]&fileMask(file+1)).PopCount() == 0
		if whiteCount > 0 && whiteLeftEmpty && whiteRightEmpty {
			dpPenalty += whiteCount * isolatedPawnPenalty
		}

		blackLeftEmpty := file == A1 || (pos.Pieces[Pawn]&pos.Colors[Black]&fileMask(file-1)).PopCount() == 0
		blackRightEmpty := file == H1 || (pos.Pieces[Pawn]&pos.Colors[Black]&fileMask(file+1)).PopCount() == 0
		if blackCount > 0 && blackLeftEmpty && blackRightEmpty {
			dpPenalty -= blackCount * isolatedPawnPenalty
		}

	}

	return eval + dpPenalty
}

func gamePhase(pos Position) int {
	var phase int

	for pt := Pawn; pt <= King; pt++ {
		pieces := pos.Pieces[pt]
		phase += pieces.PopCount() * phaseWeights[pt]
	}

	return min(phase, totalPhase)
}
