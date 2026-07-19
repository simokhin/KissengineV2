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
