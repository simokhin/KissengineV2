package main

import (
	"fmt"
	"math"

	"kissengine-bitboard/engine"
)

// pieceValueSample is roughly how many positions the piece values are
// measured on: enough that the averages are stable to a centipawn or two, and
// few enough that it takes a couple of seconds.
const pieceValueSample = 50000

// measurePieceValues returns, for each of pawn, rook, knight, bishop and queen
// (indexed by engine.PieceType), the average amount by which engine.Evaluate
// changes, from the owner's point of view, when one piece of that type is
// removed from a position. It uses whatever weights the engine currently has.
//
// This is the value SEE and move ordering want: what the evaluation itself
// thinks losing that piece is worth, including where such pieces usually stand,
// the mobility they give and the pair bonus, which none of the material or PST
// weights alone tell (and which the tuner may shuffle between them: adding a
// constant to a piece's PST and subtracting it from its material changes no
// evaluation, but would change a "value" read off the material weight).
func measurePieceValues(positions []*engine.Position) [5]float64 {
	var sum, count [5]float64

	for _, pos := range positions {
		base := engine.Evaluate(pos)

		for sq := engine.A1; sq <= engine.H8; sq++ {
			pt := pos.PieceAt(sq)
			if pt == engine.AllPieces || pt == engine.King {
				continue
			}

			color := engine.White
			if pos.Colors[engine.Black]&sq.BB() != 0 {
				color = engine.Black
			}

			without := *pos
			without.RemovePiece(sq, color, pt)

			loss := float64(base - engine.Evaluate(&without))
			if color == engine.Black {
				loss = -loss
			}

			sum[pt] += loss
			count[pt]++
		}
	}

	var values [5]float64
	for pt := range values {
		values[pt] = sum[pt] / max(count[pt], 1)
	}

	return values
}

// pieceValuesFromLines measures the piece values on about pieceValueSample of
// the given lines, taken evenly across the file, and returns them rounded to
// integers in the layout of engine's pieceValues (indexed by piece type, the
// king and the "all pieces" slot zero).
func pieceValuesFromLines(lines [][]byte) ([7]int, error) {
	step := max(1, len(lines)/pieceValueSample)

	var positions []*engine.Position
	for i := 0; i < len(lines); i += step {
		pos, _, err := parseEPDLine(lines[i])
		if err != nil {
			return [7]int{}, fmt.Errorf("line %d: %w", i+1, err)
		}
		positions = append(positions, pos)
	}

	var values [7]int
	for pt, v := range measurePieceValues(positions) {
		values[pt] = int(math.Round(v))
	}

	return values, nil
}
