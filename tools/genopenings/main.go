// Command genopenings writes a deterministic set of random opening positions
// (EPD/FEN, one per line, sorted) for engine-vs-engine matches:
//
//	go run ./tools/genopenings testdata/openings-8ply.epd
//
// Each opening is a seeded random legal walk from the start position; those
// whose static evaluation is lopsided are dropped so both colors have a fair game.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"

	"kissengine-bitboard/engine"
)

// pieceChars is indexed by engine.PieceType (Pawn, Rook, Knight, Bishop, Queen, King).
const pieceChars = "prnbqk"

func fen(p *engine.Position) string {
	var sb strings.Builder
	for rank := 7; rank >= 0; rank-- {
		empty := 0
		for file := 0; file < 8; file++ {
			sq := engine.Square(rank*8 + file)
			pt := p.PieceAt(sq)
			if pt == engine.AllPieces {
				empty++
				continue
			}
			if empty > 0 {
				sb.WriteByte(byte('0' + empty))
				empty = 0
			}
			c := pieceChars[pt]
			if p.Colors[engine.White]&(1<<uint(sq)) != 0 {
				c -= 'a' - 'A'
			}
			sb.WriteByte(c)
		}
		if empty > 0 {
			sb.WriteByte(byte('0' + empty))
		}
		if rank > 0 {
			sb.WriteByte('/')
		}
	}

	side := "w"
	if p.SideToMove == engine.Black {
		side = "b"
	}

	castling := ""
	for _, c := range []struct {
		right engine.CastlingRights
		char  string
	}{
		{engine.WhiteKingside, "K"}, {engine.WhiteQueenside, "Q"},
		{engine.BlackKingside, "k"}, {engine.BlackQueenside, "q"},
	} {
		if p.Castling&c.right != 0 {
			castling += c.char
		}
	}
	if castling == "" {
		castling = "-"
	}

	ep := "-"
	if p.EnPassant != engine.NoSquare {
		ep = fmt.Sprintf("%c%d", 'a'+int(p.EnPassant)%8, int(p.EnPassant)/8+1)
	}

	return fmt.Sprintf("%s %s %s %s %d 1", sb.String(), side, castling, ep, p.FiftyMovesRule)
}

func main() {
	count := flag.Int("n", 400, "number of distinct openings")
	plies := flag.Int("plies", 8, "random plies from the start position")
	seed := flag.Int64("seed", 12345, "random seed")
	maxEval := flag.Int("maxeval", 100, "drop openings with |static eval| above this (centipawns)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: genopenings [flags] OUT.epd")
		os.Exit(2)
	}

	rng := rand.New(rand.NewSource(*seed))
	seen := map[string]bool{}
	for len(seen) < *count {
		pos := engine.StartPos()
		ok := true
		for i := 0; i < *plies; i++ {
			var list engine.MoveList
			engine.GenerateLegalMoves(*pos, pos.SideToMove, &list)
			moves := list.Slice()
			if len(moves) == 0 {
				ok = false
				break
			}
			pos.MakeMove(moves[rng.Intn(len(moves))])
		}
		if !ok {
			continue
		}

		if e := engine.Evaluate(pos); e > *maxEval || e < -*maxEval {
			continue
		}

		// Reject positions where the side to move is already mated or stalemated.
		var list engine.MoveList
		engine.GenerateLegalMoves(*pos, pos.SideToMove, &list)
		if len(list.Slice()) == 0 {
			continue
		}

		seen[fen(pos)] = true
	}

	lines := make([]string, 0, len(seen))
	for l := range seen {
		lines = append(lines, l)
	}
	sort.Strings(lines)

	if err := os.WriteFile(flag.Arg(0), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
