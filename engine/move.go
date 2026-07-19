package engine

// From returns the origin square of the move.
func (m Move) From() Square {
	sq := (m >> 6) & 0x3F
	return Square(sq)
}

// To returns the destination square of the move.
func (m Move) To() Square {
	sq := m & 0x3F
	return Square(sq)
}

// NewMove create a Move from and origin and destination square.
func NewMove(from, to Square) Move {
	return Move(from)<<6 | Move(to)
}

// NewPromotionMove creates a Move representing a pawn promoting to the
// given piece type.
func NewPromotionMove(from, to Square, pPiece PieceType) Move {
	return Move(from)<<6 | Move(to) | Move(pPiece-1)<<12 | Move(1)<<14
}

// Promotion returns the piece type a pawn promotes to.
// Only meaningful when IsPromotion() is true.
func (m Move) Promotion() PieceType {
	return PieceType(((m >> 12) & 0x3) + 1)
}

// IsPromotion reports whether the move is a pawn promotion.
func (m Move) IsPromotion() bool {
	return (m & (1 << 14)) != 0
}
