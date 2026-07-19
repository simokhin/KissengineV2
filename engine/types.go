package engine

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

type PieceType int
type Color int
type Square int

// Move is a compact encoding of a chess move: bits 0-5 hold the destination square,
// bits 6-11 hold the origin square.
type Move uint16
