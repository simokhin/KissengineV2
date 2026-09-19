package engine

import (
	"strconv"
	"strings"
)

// StartFEN is the FEN string for the standard chess starting position.
const StartFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

// ParseFEN parses a FEN string into a Position.
func ParseFEN(fen string) *Position {
	pos := Position{}

	fields := strings.Fields(fen)

	ranks := strings.Split(fields[0], "/")
	for i, rank := range ranks {
		currentRank := 7 - i
		currentFile := 0

		for j := 0; j < len(rank); j++ {
			c := rank[j]
			if c >= '1' && c <= '8' {
				currentFile += int(c - '0')
				continue
			}

			sq := Square(currentRank*8 + currentFile)

			switch c {
			case 'p':
				pos.PutPiece(sq, Black, Pawn)
			case 'r':
				pos.PutPiece(sq, Black, Rook)
			case 'n':
				pos.PutPiece(sq, Black, Knight)
			case 'b':
				pos.PutPiece(sq, Black, Bishop)
			case 'q':
				pos.PutPiece(sq, Black, Queen)
			case 'k':
				pos.PutPiece(sq, Black, King)
			case 'P':
				pos.PutPiece(sq, White, Pawn)
			case 'R':
				pos.PutPiece(sq, White, Rook)
			case 'N':
				pos.PutPiece(sq, White, Knight)
			case 'B':
				pos.PutPiece(sq, White, Bishop)
			case 'Q':
				pos.PutPiece(sq, White, Queen)
			case 'K':
				pos.PutPiece(sq, White, King)
			}
			currentFile++
		}
	}

	switch fields[1] {
	case "w":
		pos.SideToMove = White
	case "b":
		pos.SideToMove = Black
		pos.hash ^= zobristSideToMove
	}

	for i := 0; i < len(fields[2]); i++ {
		switch fields[2][i] {
		case 'K':
			pos.Castling |= WhiteKingside
		case 'Q':
			pos.Castling |= WhiteQueenside
		case 'k':
			pos.Castling |= BlackKingside
		case 'q':
			pos.Castling |= BlackQueenside
		}
	}
	for castle := range 4 {
		if pos.Castling&CastlingRights(1<<castle) != 0 {
			pos.hash ^= zobristCastling[castle]
		}
	}

	if fields[3] == "-" {
		pos.EnPassant = NoSquare
	} else {
		pos.EnPassant = ParseSquare(fields[3])
		pos.hash ^= zobristEnPassantFile[pos.EnPassant.File()]
	}

	fiftyMovesRule, _ := strconv.Atoi(fields[4])
	pos.FiftyMovesRule = fiftyMovesRule

	return &pos
}
