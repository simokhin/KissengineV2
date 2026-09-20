package engine

// evalWeights holds every tunable evaluation weight in one flat array, so a
// term is an index plus a count (evalAcc.add) instead of a constant and an
// if in the evaluation loop. The w* constants are the indices; a block that
// says "by X" is a run of consecutive weights indexed by X.
//
// The initial values are still written as the readable tables and constants
// (pst.go, eval.go) and copied in by init below.
var evalWeights [numWeights]int

const (
	wMaterial = 0 // Pawn..Queen, by PieceType (the king has none)
	// One 64-entry table per piece type, by PieceType then by square laid out
	// from rank 8 down (index = relSq^56, see evalSide).
	wPST = wMaterial + 5
	// The king's endgame table; wPST's king table is its middlegame one and
	// evalSide blends the two by game phase.
	wKingEndgamePST = wPST + 6*64

	wKnightMobility = wKingEndgamePST + 64
	wBishopMobility = wKnightMobility + 1
	wRookMobility   = wBishopMobility + 1
	wQueenMobility  = wRookMobility + 1

	wBishopPair       = wQueenMobility + 1
	wRookOpenFile     = wBishopPair + 1
	wRookSemiOpenFile = wRookOpenFile + 1
	wPawnShield       = wRookSemiOpenFile + 1
	wDoubledPawn      = wPawnShield + 1
	wIsolatedPawn     = wDoubledPawn + 1

	wPassed        = wIsolatedPawn + 1 // by relative rank
	wPassedBlocked = wPassed + 8       // by relative rank, pawn with the square ahead occupied

	numWeights = wPassedBlocked + 8
)

func init() {
	for pt := Pawn; pt <= Queen; pt++ {
		evalWeights[wMaterial+int(pt)] = pieceValues[pt]
	}
	for pt := Pawn; pt <= King; pt++ {
		copy(evalWeights[wPST+int(pt)*64:], pst[pt][:])
	}
	copy(evalWeights[wKingEndgamePST:], kingEndgamePST[:])

	evalWeights[wKnightMobility] = knightMobilityBonus
	evalWeights[wBishopMobility] = bishopMobilityBonus
	evalWeights[wRookMobility] = rookMobilityBonus
	evalWeights[wQueenMobility] = queenMobilityBonus

	evalWeights[wBishopPair] = bishopPairBonus
	evalWeights[wRookOpenFile] = openFileBonus
	evalWeights[wRookSemiOpenFile] = semiOpenFileBonus
	evalWeights[wPawnShield] = pawnShieldBonus
	evalWeights[wDoubledPawn] = doubledPawnPenalty
	evalWeights[wIsolatedPawn] = isolatedPawnPenalty

	for rank, bonus := range passedPawnRankBonus {
		evalWeights[wPassed+rank] = bonus
		evalWeights[wPassedBlocked+rank] = bonus / blockedPassedPawnDivisor
	}
}

// evalAcc accumulates weight*count terms into a running score.
type evalAcc struct {
	score int
}

func (a *evalAcc) add(weight, count int) {
	a.score += count * evalWeights[weight]
}
