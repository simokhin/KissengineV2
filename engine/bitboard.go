package engine

import "math/bits"

type Bitboard uint64

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
