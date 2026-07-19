package engine

// knightOffset represent a single (file, rank) delta describing one of a knight's eight move shapes.
type knightOffset struct {
	df int
	dr int
}

// kingOffset represents a single (file, rank) delta describing one of a king's
// eight move directions.
type kingOffset struct {
	f int
	r int
}

// kingOffsets holds all eight possible king move deltas.
var kingOffsets = []kingOffset{
	{
		f: 1,
		r: 0,
	},
	{
		f: 1,
		r: 1,
	},
	{
		f: 0,
		r: 1,
	},
	{
		f: -1,
		r: 1,
	},
	{
		f: -1,
		r: 0,
	},
	{
		f: -1,
		r: -1,
	},
	{
		f: 0,
		r: -1,
	},
	{
		f: 1,
		r: -1,
	},
}

// knightOffsets holds all eight possible knight move deltas.
var knightOffsets = []knightOffset{
	{
		df: 1,
		dr: 2,
	},
	{
		df: 2,
		dr: 1,
	},
	{
		df: 2,
		dr: -1,
	},
	{
		df: 1,
		dr: -2,
	},
	{
		df: -1,
		dr: -2,
	},
	{
		df: -2,
		dr: -1,
	},
	{
		df: -2,
		dr: 1,
	},
	{
		df: -1,
		dr: 2,
	},
}

// KnightAttacks is a precomputed table of knight attack bitboards indexed by square.
var KnightAttacks [64]Bitboard

// KingAttacks is a precomputed table of king attack bitboards indexed by square.
var KingAttacks [64]Bitboard

// GenerateKingMoves returns all pseudo-legal king moves for the given color in the position.
func GenerateKingMoves(pos Position, color Color) []Move {
	var moves []Move
	king := pos.Pieces[King] & pos.Colors[color]

	from := king.PopLSB()
	kingAttacks := KingAttacks[from] & ^pos.Colors[color]

	for kingAttacks != 0 {
		to := kingAttacks.PopLSB()
		m := NewMove(from, to)
		moves = append(moves, m)
	}

	return moves
}

// GenerateKnightMoves returns all pseudo-legal knight moves for the given color in the position.
func GenerateKnightMoves(pos Position, color Color) []Move {
	var moves []Move
	knights := pos.Pieces[Knight] & pos.Colors[color]

	for knights != 0 {
		from := knights.PopLSB()
		attacks := KnightAttacks[from] & ^pos.Colors[color]

		for attacks != 0 {
			to := attacks.PopLSB()
			m := NewMove(from, to)
			moves = append(moves, m)
		}
	}

	return moves
}

// KnightAttacksFrom returns the bitboard of squares a knight on square s can attack.
func KnightAttacksFrom(s Square) Bitboard {
	var attacks Bitboard

	for _, offset := range knightOffsets {
		newFile := s.File() + offset.df
		newRank := s.Rank() + offset.dr
		if onBoard(newFile, newRank) {
			sq := newRank*8 + newFile
			attacks |= Square(sq).BB()
		}
	}

	return attacks
}

// KingAttacksFrom returns the bitboard of squares a king on square s can attack.
func KingAttacksFrom(s Square) Bitboard {
	var attacks Bitboard

	for _, offset := range kingOffsets {
		newFile := s.File() + offset.f
		newRank := s.Rank() + offset.r
		if onBoard(newFile, newRank) {
			sq := newRank*8 + newFile
			attacks |= Square(sq).BB()
		}
	}

	return attacks
}

// init populates the knightAttacks and kingAttacks table for all 64 squares.
func init() {
	for i := range KnightAttacks {
		KnightAttacks[i] = KnightAttacksFrom(Square(i))
	}

	for i := range KingAttacks {
		KingAttacks[i] = KingAttacksFrom(Square(i))
	}
}
