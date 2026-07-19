package engine

// rookRelevantMask returns the squares whose occupancy affect a rook's attacks from s,
// excuding board edges.
func rookRelevantMask(s Square) Bitboard {
	return (rookAttacksNorth(s, 0) & ^Rank8) | (rookAttacksSouth(s, 0) & ^Rank1) |
		(rookAttacksEast(s, 0) & ^FileH) | (rookAttacksWest(s, 0) & ^FileA)
}

// bishopRelevantMask returns the squares whose occupancy affects a bishop's attacks from s,
// excluding board edges.
func bishopRelevantMask(s Square) Bitboard {
	return (bishopAttacksNorthEast(s, 0) & ^Rank8 & ^FileH) |
		(bishopAttacksNorthWest(s, 0) & ^Rank8 & ^FileA) |
		(bishopAttacksSouthEast(s, 0) & ^Rank1 & ^FileH) |
		(bishopAttacksSouthWest(s, 0) & ^Rank1 & ^FileA)
}

// queenAttacks returns the squares a queen on s attacks
// given the occupied squares on the board.
func queenAttacks(s Square, occupied Bitboard) Bitboard {
	return rookAttacks(s, occupied) | bishopAttacks(s, occupied)
}

// bishopAttacksNorthEast returns the squares a bishop on s attacks moving north-east,
// stopping at (and including) the first blocker.
func bishopAttacksNorthEast(s Square, occupied Bitboard) Bitboard {
	var attacks Bitboard
	current := s

	for current.Rank() < 7 && current.File() < 7 {
		next := current + 9
		attacks |= next.BB()
		if occupied&next.BB() != 0 {
			break
		}
		current = next
	}

	return attacks
}

// bishopAttacksNorthWest returns the squares a bishop on s attacks moving north-west,
// stopping at (and including) the first blocker.
func bishopAttacksNorthWest(s Square, occupied Bitboard) Bitboard {
	var attacks Bitboard
	current := s

	for current.Rank() < 7 && current.File() > 0 {
		next := current + 7
		attacks |= next.BB()
		if occupied&next.BB() != 0 {
			break
		}
		current = next
	}

	return attacks
}

// bishopAttacksSouthEast returns the squares a bishop on s attacks moving south-east,
// stopping at (and including) the first blocker.
func bishopAttacksSouthEast(s Square, occupied Bitboard) Bitboard {
	var attacks Bitboard
	current := s

	for current.Rank() > 0 && current.File() < 7 {
		next := current - 7
		attacks |= next.BB()
		if occupied&next.BB() != 0 {
			break
		}
		current = next
	}

	return attacks
}

// bishopAttacksSouthWest returns the squares a bishop on s attacks moving south-west,
// stopping at (and including) the first blocker.
func bishopAttacksSouthWest(s Square, occupied Bitboard) Bitboard {
	var attacks Bitboard
	current := s

	for current.Rank() > 0 && current.File() > 0 {
		next := current - 9
		attacks |= next.BB()
		if occupied&next.BB() != 0 {
			break
		}
		current = next
	}

	return attacks
}

// bishopAttacks returns the squares a bishop on s attacks given the occupied squares on the board.
func bishopAttacks(s Square, occupied Bitboard) Bitboard {
	return bishopAttacksNorthEast(s, occupied) | bishopAttacksNorthWest(s, occupied) | bishopAttacksSouthEast(s, occupied) | bishopAttacksSouthWest(s, occupied)
}

// rookAttacks returns the squares a rook on s attacks given the occupied squares
// on the boards.
func rookAttacks(s Square, occupied Bitboard) Bitboard {
	return rookAttacksNorth(s, occupied) | rookAttacksSouth(s, occupied) | rookAttacksWest(s, occupied) | rookAttacksEast(s, occupied)
}

// rookAttacksNorth returns the squares a rook on s attacks moveing north,
// stopping at (and including) the first blocker.
func rookAttacksNorth(s Square, occupied Bitboard) Bitboard {
	var attacks Bitboard
	current := s

	for current.Rank() < 7 {
		next := current + 8
		attacks |= next.BB()
		if occupied&next.BB() != 0 {
			break
		}
		current = next
	}

	return attacks
}

// rookAttacksSouth returns the squares a rook on s attacks moveing south,
// stopping at (and including) the first blocker.
func rookAttacksSouth(s Square, occupied Bitboard) Bitboard {
	var attacks Bitboard
	current := s

	for current.Rank() > 0 {
		next := current - 8
		attacks |= next.BB()
		if occupied&next.BB() != 0 {
			break
		}
		current = next
	}

	return attacks
}

// rookAttacksWest returns the squares a rook on s attacks moving west,
// stopping at (and including) the first blocker.
func rookAttacksWest(s Square, occupied Bitboard) Bitboard {
	var attacks Bitboard
	current := s

	for current.File() > 0 {
		next := current - 1
		attacks |= next.BB()
		if occupied&next.BB() != 0 {
			break
		}
		current = next
	}

	return attacks
}

// rookAttacksEast returns the squares a rook on s attacks moving east,
// stopping at (and including) the first blocker.
func rookAttacksEast(s Square, occupied Bitboard) Bitboard {
	var attacks Bitboard
	current := s

	for current.File() < 7 {
		next := current + 1
		attacks |= next.BB()
		if occupied&next.BB() != 0 {
			break
		}
		current = next
	}

	return attacks
}
