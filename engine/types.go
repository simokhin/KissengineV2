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

const (
	WhiteKingside CastlingRights = 1 << iota
	WhiteQueenside
	BlackKingside
	BlackQueenside
)

type PieceType int
type Color int
type Square int

// CastlingRights is a bitmask of which castling moves still available.
type CastlingRights uint8

// File returns the file (0-7, a-h) of the square.
func (s Square) File() int {
	return int(s % 8)
}

// Rank returns the rank (0-7, 1-8) of the square.
func (s Square) Rank() int {
	return int(s / 8)
}

// onBoard reports whether the given file and rank are within the bounds of the chessboard.
func onBoard(file, rank int) bool {
	return file >= 0 && file <= 7 && rank >= 0 && rank <= 7
}

// Rank1 is a bitboard with all squares of the first rank set.
var Rank1 Bitboard

// Rank3 is a bitboard with all squares of the third rank set.
var Rank3 Bitboard

// Rank6 is a bitboard with all squares of the sixth rank set.
var Rank6 Bitboard

// Rank8 is a bitboard with all squares of the eighth rank set.
var Rank8 Bitboard

// FileA is a bitboard with all squares of the a-file set.
var FileA Bitboard

// FileH is a bitboard with all squares of the h-file set.
var FileH Bitboard

// initRankFileMasks populates the Rank1, Rank3, Rank6, Rank8, FileA, FileH
// masks. Called explicitly (not as its own init()) so callers that need
// these masks during their own init() can control the ordering instead of
// relying on Go's alphabetical-by-filename init() order across files.
func initRankFileMasks() {
	Rank1 = rankMask(A1)
	Rank3 = rankMask(A3)
	Rank6 = rankMask(A6)
	Rank8 = rankMask(A8)

	FileA = fileMask(A1)
	FileH = fileMask(H1)
}

func init() {
	initRankFileMasks()
}
