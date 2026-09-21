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
// The values come from tunedWeights (eval_params.go, written by tools/texel),
// looked up by WeightName, and nowhere else: there are no hand-written tables
// behind it. A weight with no entry there, such as a feature added since the
// last tuning run, starts at bootstrapWeights or at zero.
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

// bootstrapWeights are the starting values of weights that tunedWeights has no
// entry for: zero for everything except material, so that an untuned
// evaluation still counts pieces and tools/texel has something to fit K to.
var bootstrapWeights = map[string]weight{
	"material/pawn":   {100, 100},
	"material/rook":   {500, 500},
	"material/knight": {320, 320},
	"material/bishop": {330, 330},
	"material/queen":  {900, 900},
}

func init() {
	for i := range evalWeights {
		name := WeightName(i)
		if w, ok := tunedWeights[name]; ok {
			evalWeights[i] = w
		} else {
			evalWeights[i] = bootstrapWeights[name]
		}
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
