package engine

// knightOffset represent a single (file, rank) delta describing one of a knight's eight move shapes.
type knightOffset struct {
	df int
	dr int
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

// KnightAttacksFrom returns the bitboard of squares a knight on square s can attack.
func KnightAttacksFrom(s Square) Bitboard {
	var attacks Bitboard

	for _, offset := range knightOffsets {
		newFile := s.File() + offset.df
		newRank := s.Rank() + offset.dr
		if (newFile >= 0 && newFile <= 7) && (newRank >= 0 && newRank <= 7) {
			sq := newRank*8 + newFile
			attacks |= Square(sq).BB()
		}
	}

	return attacks
}
