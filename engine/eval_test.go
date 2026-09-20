package engine

import (
	"math/rand"
	"testing"
)

// mirrorPosition flips the board vertically and swaps the colors (and the
// side to move, castling rights and en passant square with them), so the
// result is the same game situation seen from the other side.
func mirrorPosition(pos *Position) *Position {
	m := &Position{EnPassant: NoSquare, FiftyMovesRule: pos.FiftyMovesRule}

	for sq := A1; sq <= H8; sq++ {
		pt := pos.PieceAt(sq)
		if pt == AllPieces {
			continue
		}
		c := White
		if pos.Colors[Black]&sq.BB() != 0 {
			c = Black
		}
		m.PutPiece(sq^56, c^1, pt)
	}

	m.SideToMove = pos.SideToMove ^ 1
	if pos.EnPassant != NoSquare {
		m.EnPassant = pos.EnPassant ^ 56
	}
	if pos.Castling&WhiteKingside != 0 {
		m.Castling |= BlackKingside
	}
	if pos.Castling&WhiteQueenside != 0 {
		m.Castling |= BlackQueenside
	}
	if pos.Castling&BlackKingside != 0 {
		m.Castling |= WhiteKingside
	}
	if pos.Castling&BlackQueenside != 0 {
		m.Castling |= WhiteQueenside
	}

	return m
}

// evalTestPositions calls visit for every position of a few reference perft
// trees, of some hand-picked endgames (passed, blocked and isolated pawns,
// bishop pairs, open files) and of seeded random games, which is what
// actually reaches middlegames and endgames with a full mix of pieces.
func evalTestPositions(t *testing.T, visit func(pos *Position)) {
	t.Helper()

	var walk func(pos *Position, depth int)
	walk = func(pos *Position, depth int) {
		visit(pos)
		if depth == 0 {
			return
		}

		var moves MoveList
		GenerateLegalMoves(*pos, pos.SideToMove, &moves)
		for _, m := range moves.Slice() {
			undo := pos.MakeMove(m)
			walk(pos, depth-1)
			pos.UnmakeMove(m, undo)
		}
	}

	fens := []string{
		StartFEN,
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
		"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1",
		"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8",
		"r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10",
	}
	for _, fen := range fens {
		walk(ParseFEN(fen), 2)
	}

	endgames := []string{
		"8/5pk1/6p1/3P4/8/1P4P1/5PK1/8 w - - 0 1",
		"8/P7/8/8/8/8/p7/k1K5 w - - 0 1",
		"8/3k4/8/3pP3/3P4/8/3K4/8 w - - 0 1",
		"4k3/8/3p4/3P4/8/8/PP3PPP/4K3 b - - 0 1",
		"2b1kb2/8/8/8/8/8/8/2B1KB2 w - - 0 1",
		"r3k2r/8/8/8/8/8/8/R3K2R b KQkq - 0 1",
		"4k3/1p6/8/P7/P7/8/8/4K3 w - - 0 1",
		"3rk3/3p4/8/8/8/8/3P4/3RK3 w - - 0 1",
	}
	for _, fen := range endgames {
		walk(ParseFEN(fen), 2)
	}

	rng := rand.New(rand.NewSource(1))
	for range 300 {
		pos := ParseFEN(StartFEN)
		for range 160 {
			var moves MoveList
			GenerateLegalMoves(*pos, pos.SideToMove, &moves)
			if len(moves.Slice()) == 0 {
				break
			}
			pos.MakeMove(moves.Slice()[rng.Intn(len(moves.Slice()))])
			visit(pos)
		}
	}
}

// TestEvaluateSymmetry checks that Evaluate has no color bias: the mirrored
// position (colors swapped, board flipped) must score exactly the negation.
func TestEvaluateSymmetry(t *testing.T) {
	n := 0
	evalTestPositions(t, func(pos *Position) {
		n++
		got, mirrored := Evaluate(pos), Evaluate(mirrorPosition(pos))
		if got != -mirrored {
			t.Fatalf("Evaluate = %d but mirrored position gives %d (want %d)", got, mirrored, -got)
		}
	})
	t.Logf("checked %d positions", n)
}
