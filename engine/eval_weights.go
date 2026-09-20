package engine

// weight is one tunable evaluation weight as a middlegame/endgame pair;
// Evaluate blends the two sums by game phase.
type weight struct {
	mg, eg int
}

// evalWeights holds every tunable evaluation weight in one flat array, so a
// term is an index plus a count (evalAcc.add) instead of a constant and an
// if in the evaluation loop. The w* constants are the indices; a block that
// says "by X" is a run of consecutive weights indexed by X.
//
// The initial values are still written as the readable tables and constants
// (pst.go, eval.go) and copied in by init below.
var evalWeights [numWeights]weight

const (
	wMaterial = 0 // Pawn..Queen, by PieceType (the king has none)
	// One 64-entry table per piece type, by PieceType then by square laid out
	// from rank 8 down (index = relSq^56, see evalSide).
	wPST = wMaterial + 5

	wKnightMobility = wPST + 6*64
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
	// Everything starts out the same in the middlegame and the endgame except
	// the king's table, which has a separate endgame version.
	both := func(v int) weight { return weight{v, v} }

	for pt := Pawn; pt <= Queen; pt++ {
		evalWeights[wMaterial+int(pt)] = both(pieceValues[pt])
	}
	for pt := Pawn; pt <= King; pt++ {
		for sq, v := range pst[pt] {
			evalWeights[wPST+int(pt)*64+sq] = both(v)
		}
	}
	for sq, v := range kingEndgamePST {
		evalWeights[wPST+int(King)*64+sq].eg = v
	}

	evalWeights[wKnightMobility] = both(knightMobilityBonus)
	evalWeights[wBishopMobility] = both(bishopMobilityBonus)
	evalWeights[wRookMobility] = both(rookMobilityBonus)
	evalWeights[wQueenMobility] = both(queenMobilityBonus)

	evalWeights[wBishopPair] = both(bishopPairBonus)
	evalWeights[wRookOpenFile] = both(openFileBonus)
	evalWeights[wRookSemiOpenFile] = both(semiOpenFileBonus)
	evalWeights[wPawnShield] = both(pawnShieldBonus)
	evalWeights[wDoubledPawn] = both(doubledPawnPenalty)
	evalWeights[wIsolatedPawn] = both(isolatedPawnPenalty)

	for rank, bonus := range passedPawnRankBonus {
		evalWeights[wPassed+rank] = both(bonus)
		evalWeights[wPassedBlocked+rank] = both(bonus / blockedPassedPawnDivisor)
	}
}

// evalAcc accumulates weight*count terms into separate middlegame and
// endgame sums.
type evalAcc struct {
	mg, eg int
	// trace, when non-nil, also records how many times each weight was
	// counted; it is nil during search, so this costs one predictable branch.
	trace *[numWeights]int16
}

func (a *evalAcc) add(idx, count int) {
	w := &evalWeights[idx]
	a.mg += count * w.mg
	a.eg += count * w.eg
	if a.trace != nil {
		a.trace[idx] += int16(count)
	}
}
