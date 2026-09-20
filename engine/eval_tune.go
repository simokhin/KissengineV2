package engine

import "strconv"

// This file is the interface between the evaluation and the Texel tuner
// (tools/texel): the tuner never calls Evaluate on its training positions.
// It calls Trace once per position to learn how many times each weight was
// counted, and from then on only multiplies those counts by candidate weights.

// NumWeights is the length of the weight vector: Weights returns this many
// pairs and Trace indices are below it.
const NumWeights = numWeights

// Weights returns a copy of the current weights as {middlegame, endgame} pairs,
// indexed like TraceEntry.Index.
func Weights() [][2]int {
	w := make([][2]int, numWeights)
	for i, v := range evalWeights {
		w[i] = [2]int{v.mg, v.eg}
	}
	return w
}

// TraceEntry says that a weight was counted Count times more for White than
// for Black.
type TraceEntry struct {
	Index uint16
	Count int16
}

// Trace explains Evaluate(pos) as a sum over weights: the returned entries
// (only those with a nonzero Count) and the game phase satisfy
//
//	mg = Σ Count·Weights()[Index][0]
//	eg = Σ Count·Weights()[Index][1]
//	Evaluate(pos) = (mg·phase + eg·(24−phase)) / 24
//
// exactly, with the integer division of Evaluate.
func Trace(pos *Position) (entries []TraceEntry, phase int) {
	var white, black [numWeights]int16
	evaluate(pos, &white, &black)

	for i := range numWeights {
		if count := white[i] - black[i]; count != 0 {
			entries = append(entries, TraceEntry{Index: uint16(i), Count: count})
		}
	}

	return entries, gamePhase(pos)
}

var pieceNames = [...]string{Pawn: "pawn", Rook: "rook", Knight: "knight", Bishop: "bishop", Queen: "queen", King: "king"}

// WeightName returns a readable name for weight i, such as "pst/knight/e4"
// (the square as seen from the owner's side), "material/queen" or
// "passed-blocked/rank6" (relative rank, 1-based).
func WeightName(i int) string {
	switch {
	case i < 0 || i >= numWeights:
		panic("WeightName: index out of range: " + strconv.Itoa(i))
	case i < wPST:
		return "material/" + pieceNames[i-wMaterial]
	case i < wKnightMobility:
		p := i - wPST
		return "pst/" + pieceNames[p/64] + "/" + Square((p%64)^56).String()
	case i == wKnightMobility:
		return "mobility/knight"
	case i == wBishopMobility:
		return "mobility/bishop"
	case i == wRookMobility:
		return "mobility/rook"
	case i == wQueenMobility:
		return "mobility/queen"
	case i == wBishopPair:
		return "bishop-pair"
	case i == wRookOpenFile:
		return "rook-open-file"
	case i == wRookSemiOpenFile:
		return "rook-semi-open-file"
	case i == wPawnShield:
		return "pawn-shield"
	case i == wDoubledPawn:
		return "doubled-pawn"
	case i == wIsolatedPawn:
		return "isolated-pawn"
	case i < wPassedBlocked:
		return "passed/rank" + strconv.Itoa(i-wPassed+1)
	default:
		return "passed-blocked/rank" + strconv.Itoa(i-wPassedBlocked+1)
	}
}
