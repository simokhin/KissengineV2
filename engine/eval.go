package engine

const totalPhase = 24

const (
	knightMobilityBonus = 4
	bishopMobilityBonus = 3
	rookMobilityBonus   = 2
	queenMobilityBonus  = 1

	bishopPairBonus = 30

	openFileBonus     = 15
	semiOpenFileBonus = 8

	pawnShieldBonus = 10
)

const (
	doubledPawnPenalty  = -10
	isolatedPawnPenalty = -15
)

var passedPawnRankBonus = [8]int{0, 5, 10, 20, 35, 60, 100, 150}

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

// fileMasks[file] is a precomputed bitboard of an entire file (0=A..7=H).
var fileMasks [8]Bitboard

// passedPawnMasks[color][sq] is a precomputed bitboard covering the pawn's
// own file and the two adjacent files, on all ranks ahead of it (toward
// promotion for that color). A pawn is passed if no enemy pawn intersects
// this mask -- computing it once at startup avoids rebuilding it out of
// fileMask/rankMask loops on every Evaluate call.
var passedPawnMasks [2][64]Bitboard

// kingShieldMasks[color][sq] is a precomputed bitboard covering the king's
// own file and the two adjacent files, on the single rank directly ahead of
// it (toward the enemy). Empty if the king is already on its own back rank
// edge (no rank ahead to shield with).
var kingShieldMasks [2][64]Bitboard

func init() {
	for f := range 8 {
		fileMasks[f] = fileMask(Square(f))
	}

	for sq := A1; sq <= H8; sq++ {
		frontFiles := fileMasks[sq.File()]
		if sq.File() > 0 {
			frontFiles |= fileMasks[sq.File()-1]
		}
		if sq.File() < 7 {
			frontFiles |= fileMasks[sq.File()+1]
		}

		var whiteFrontRanks Bitboard
		for r := sq.Rank() + 1; r <= 7; r++ {
			whiteFrontRanks |= rankMask(Square(r * 8))
		}
		passedPawnMasks[White][sq] = frontFiles & whiteFrontRanks

		var blackFrontRanks Bitboard
		for r := sq.Rank() - 1; r >= 0; r-- {
			blackFrontRanks |= rankMask(Square(r * 8))
		}
		passedPawnMasks[Black][sq] = frontFiles & blackFrontRanks

		if sq.Rank() < 7 {
			kingShieldMasks[White][sq] = frontFiles & rankMask(Square((sq.Rank()+1)*8))
		}
		if sq.Rank() > 0 {
			kingShieldMasks[Black][sq] = frontFiles & rankMask(Square((sq.Rank()-1)*8))
		}
	}
}

func Evaluate(pos *Position) int {
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

			// Rook on open file bonus
			var ofBonus int
			if pt == Rook {
				frontFile := fileMasks[sq.File()]
				if (pos.Pieces[Pawn] & frontFile) == 0 {
					ofBonus += openFileBonus
				} else if (pos.Pieces[Pawn] & pos.Colors[White] & frontFile) == 0 {
					ofBonus += semiOpenFileBonus
				}
			}

			// King safety bonus
			var ksBonus int
			if pt == King {
				ksBonus = (pos.Pieces[Pawn] & pos.Colors[White] & kingShieldMasks[White][sq]).PopCount() * pawnShieldBonus
			}

			// Passed pawn bonus
			var passedBonus int
			if pt == Pawn && pos.Pieces[Pawn]&pos.Colors[Black]&passedPawnMasks[White][sq] == 0 {
				passedBonus = passedPawnRankBonus[sq.Rank()]
			}

			eval += pieceValues[pt] + pstValue + mobilityBonus + passedBonus + ofBonus + ksBonus
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

			// Rook on open file bonus
			var ofBonus int
			if pt == Rook {
				frontFile := fileMasks[sq.File()]
				if (pos.Pieces[Pawn] & frontFile) == 0 {
					ofBonus += openFileBonus
				} else if (pos.Pieces[Pawn] & pos.Colors[Black] & frontFile) == 0 {
					ofBonus += semiOpenFileBonus
				}
			}

			// King safety bonus
			var ksBonus int
			if pt == King {
				ksBonus = (pos.Pieces[Pawn] & pos.Colors[Black] & kingShieldMasks[Black][sq]).PopCount() * pawnShieldBonus
			}

			// Passed pawn bonus
			var passedBonus int
			if pt == Pawn && pos.Pieces[Pawn]&pos.Colors[White]&passedPawnMasks[Black][sq] == 0 {
				passedBonus = passedPawnRankBonus[7-sq.Rank()]
			}

			eval -= pieceValues[pt] + pstValue + mobilityBonus + passedBonus + ofBonus + ksBonus
		}
	}

	// Doubled/isolated pawn penalty
	var dpPenalty int
	for f := range 8 {

		// Check if pawn is doubled
		whiteCount := (pos.Pieces[Pawn] & pos.Colors[White] & fileMasks[f]).PopCount()
		if whiteCount > 1 {
			dpPenalty += (whiteCount - 1) * doubledPawnPenalty
		}

		blackCount := (pos.Pieces[Pawn] & pos.Colors[Black] & fileMasks[f]).PopCount()
		if blackCount > 1 {
			dpPenalty -= (blackCount - 1) * doubledPawnPenalty
		}

		// Check if pawn is isolated
		whiteLeftEmpty := f == 0 || (pos.Pieces[Pawn]&pos.Colors[White]&fileMasks[f-1]).PopCount() == 0
		whiteRightEmpty := f == 7 || (pos.Pieces[Pawn]&pos.Colors[White]&fileMasks[f+1]).PopCount() == 0
		if whiteCount > 0 && whiteLeftEmpty && whiteRightEmpty {
			dpPenalty += whiteCount * isolatedPawnPenalty
		}

		blackLeftEmpty := f == 0 || (pos.Pieces[Pawn]&pos.Colors[Black]&fileMasks[f-1]).PopCount() == 0
		blackRightEmpty := f == 7 || (pos.Pieces[Pawn]&pos.Colors[Black]&fileMasks[f+1]).PopCount() == 0
		if blackCount > 0 && blackLeftEmpty && blackRightEmpty {
			dpPenalty -= blackCount * isolatedPawnPenalty
		}
	}

	// Bishop pair bonus
	var bpBonus int
	if (pos.Pieces[Bishop] & pos.Colors[White]).PopCount() >= 2 {
		bpBonus += bishopPairBonus
	}
	if (pos.Pieces[Bishop] & pos.Colors[Black]).PopCount() >= 2 {
		bpBonus -= bishopPairBonus
	}

	return eval + dpPenalty + bpBonus
}

func gamePhase(pos *Position) int {
	var phase int

	for pt := Pawn; pt <= King; pt++ {
		pieces := pos.Pieces[pt]
		phase += pieces.PopCount() * phaseWeights[pt]
	}

	return min(phase, totalPhase)
}
