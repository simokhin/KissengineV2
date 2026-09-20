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

// blockedPassedPawnDivisor divides a passed pawn's bonus while the square in
// front of it is occupied: a blockaded pawn can't advance, so it is worth much
// less than a free one (at rank 6/7 the full bonus once made the engine sell a
// queen for a pawn that then sat blocked and was lost).
const blockedPassedPawnDivisor = 2

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

// Evaluate scores the position from White's perspective: what White has
// minus what Black has, both counted by evalSide.
func Evaluate(pos *Position) int {
	phase := gamePhase(pos)

	return evalSide(pos, White, phase) - evalSide(pos, Black, phase)
}

// evalSide sums every evaluation term for the pieces of color us; penalties
// come out negative. It is written once from the mover's point of view, so
// White and Black can't drift apart: squares are mapped through orient and
// "ahead" through pawnPush.
func evalSide(pos *Position, us Color, phase int) int {
	own := pos.Colors[us]
	ourPawns := pos.Pieces[Pawn] & own
	theirPawns := pos.Pieces[Pawn] & pos.Colors[us^1]
	occupied := pos.Pieces[AllPieces]

	// orient maps a square to the same square as seen from White's side
	// (mirrored vertically for Black), so the piece-square tables and the
	// passed-pawn ranks can be indexed the same way for both colors.
	// pawnPush is the square offset one step towards promotion.
	orient, pawnPush := Square(0), Square(8)
	if us == Black {
		orient, pawnPush = 56, -8
	}

	var eval int

	for pt := Pawn; pt <= King; pt++ {
		pieces := pos.Pieces[pt] & own
		for pieces != 0 {
			sq := pieces.PopLSB()
			relSq := sq ^ orient // the PSTs are laid out from rank 8 down, hence ^56 below

			// Add PST value for all pieces
			pstValue := pst[pt][relSq^56]

			// Change PST for King in the endgame
			if pt == King {
				pstValue = (pst[King][relSq^56]*phase + kingEndgamePST[relSq^56]*(totalPhase-phase)) / totalPhase
			}

			// Mobility bonus
			mobilityBonus := 0
			switch pt {
			case Knight:
				mobilityBonus = (KnightAttacks[sq] &^ own).PopCount() * knightMobilityBonus
			case Bishop:
				mobilityBonus = (bishopAttacksMagic(sq, occupied) &^ own).PopCount() * bishopMobilityBonus
			case Rook:
				mobilityBonus = (rookAttacksMagic(sq, occupied) &^ own).PopCount() * rookMobilityBonus
			case Queen:
				mobilityBonus = (queenAttacksMagic(sq, occupied) &^ own).PopCount() * queenMobilityBonus
			}

			// Rook on open file bonus
			var ofBonus int
			if pt == Rook {
				file := fileMasks[sq.File()]
				if (pos.Pieces[Pawn] & file) == 0 {
					ofBonus += openFileBonus
				} else if (ourPawns & file) == 0 {
					ofBonus += semiOpenFileBonus
				}
			}

			// King safety bonus
			var ksBonus int
			if pt == King {
				ksBonus = (ourPawns & kingShieldMasks[us][sq]).PopCount() * pawnShieldBonus
			}

			// Passed pawn bonus
			var passedBonus int
			if pt == Pawn && theirPawns&passedPawnMasks[us][sq] == 0 {
				passedBonus = passedPawnRankBonus[relSq.Rank()]
				if occupied&(sq+pawnPush).BB() != 0 {
					passedBonus /= blockedPassedPawnDivisor
				}
			}

			eval += pieceValues[pt] + pstValue + mobilityBonus + passedBonus + ofBonus + ksBonus
		}
	}

	// Doubled/isolated pawn penalty
	for f := range 8 {

		// Check if pawn is doubled
		count := (ourPawns & fileMasks[f]).PopCount()
		if count > 1 {
			eval += (count - 1) * doubledPawnPenalty
		}

		// Check if pawn is isolated
		leftEmpty := f == 0 || (ourPawns&fileMasks[f-1]).PopCount() == 0
		rightEmpty := f == 7 || (ourPawns&fileMasks[f+1]).PopCount() == 0
		if count > 0 && leftEmpty && rightEmpty {
			eval += count * isolatedPawnPenalty
		}
	}

	// Bishop pair bonus
	if (pos.Pieces[Bishop] & own).PopCount() >= 2 {
		eval += bishopPairBonus
	}

	return eval
}

func gamePhase(pos *Position) int {
	var phase int

	for pt := Pawn; pt <= King; pt++ {
		pieces := pos.Pieces[pt]
		phase += pieces.PopCount() * phaseWeights[pt]
	}

	return min(phase, totalPhase)
}
