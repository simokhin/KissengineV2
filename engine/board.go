package engine

import "math/bits"

const (
	White Color = iota
	Black
)

const (
	Pawn PieceType = iota
	Rook
	Knight
	Bishop
	Queen
	King
	AllPieces
)

// Squares
const (
	A1 Square = iota
	B1
	C1
	D1
	E1
	F1
	G1
	H1
	A2
	B2
	C2
	D2
	E2
	F2
	G2
	H2
	A3
	B3
	C3
	D3
	E3
	F3
	G3
	H3
	A4
	B4
	C4
	D4
	E4
	F4
	G4
	H4
	A5
	B5
	C5
	D5
	E5
	F5
	G5
	H5
	A6
	B6
	C6
	D6
	E6
	F6
	G6
	H6
	A7
	B7
	C7
	D7
	E7
	F7
	G7
	H7
	A8
	B8
	C8
	D8
	E8
	F8
	G8
	H8
	NoSquare
)

type Bitboard uint64
type PieceType int
type Color int
type Square int

type Position struct {
	Pieces [7]Bitboard
	Colors [2]Bitboard
}

// PopCount returns the number of set bits in the bitboard.
func (b Bitboard) PopCount() int {
	return bits.OnesCount64(uint64(b))
}

// LSB returns the index of the least significant set bit.
func (b Bitboard) LSB() Square {
	return Square(bits.TrailingZeros64(uint64(b)))
}

// PopLSB returns the index of the least significant set bit and clears it.
func (b *Bitboard) PopLSB() Square {
	sq := b.LSB()
	*b &= *b - 1
	return sq
}

// BB returns a bitboard with only the bit at square s set.
func (s Square) BB() Bitboard {
	return Bitboard(1) << s
}

// PutPiece places a piece of the given color and type on square s.
func (p *Position) PutPiece(s Square, c Color, pt PieceType) {
	p.Pieces[pt] |= s.BB()
	p.Colors[c] |= s.BB()
	p.Pieces[AllPieces] |= s.BB()
}

func StartPos() *Position {
	position := Position{}

	// White pawns
	for s := A2; s <= H2; s++ {
		position.PutPiece(s, White, Pawn)
	}

	// Black pawns
	for s := A7; s <= H7; s++ {
		position.PutPiece(s, Black, Pawn)
	}

	// Rooks
	position.PutPiece(H1, White, Rook)
	position.PutPiece(A1, White, Rook)
	position.PutPiece(A8, Black, Rook)
	position.PutPiece(H8, Black, Rook)

	// Knights
	position.PutPiece(B1, White, Knight)
	position.PutPiece(G1, White, Knight)
	position.PutPiece(B8, Black, Knight)
	position.PutPiece(G8, Black, Knight)

	// Bishops
	position.PutPiece(C1, White, Bishop)
	position.PutPiece(F1, White, Bishop)
	position.PutPiece(C8, Black, Bishop)
	position.PutPiece(F8, Black, Bishop)

	// Queen
	position.PutPiece(D1, White, Queen)
	position.PutPiece(D8, Black, Queen)

	// King
	position.PutPiece(E1, White, King)
	position.PutPiece(E8, Black, King)

	return &position
}
