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

// GenerateMoves returns all pseudo-legal moves for the given color in the position.
func GenerateMoves(pos Position, color Color) []Move {
	var moves []Move

	pawnMoves := GeneratePawnMoves(pos, color)
	moves = append(moves, pawnMoves...)

	kingMoves := GenerateKingMoves(pos, color)
	moves = append(moves, kingMoves...)

	knightMoves := GenerateKnightMoves(pos, color)
	moves = append(moves, knightMoves...)

	bishopMoves := GenerateBishopMoves(pos, color)
	moves = append(moves, bishopMoves...)

	rookMoves := GenerateRookMoves(pos, color)
	moves = append(moves, rookMoves...)

	queenMoves := GenerateQueenMoves(pos, color)
	moves = append(moves, queenMoves...)

	return moves
}

// GenerateQueenMoves returns all pseudo-legal queen moves for the given color
// in the position
func GenerateQueenMoves(pos Position, color Color) []Move {
	var moves []Move
	queen := pos.Pieces[Queen] & pos.Colors[color]

	for queen != 0 {
		from := queen.PopLSB()
		attacks := queenAttacksMagic(from, pos.Pieces[AllPieces]) & ^pos.Colors[color]

		for attacks != 0 {
			to := attacks.PopLSB()
			m := NewMove(from, to)
			moves = append(moves, m)
		}
	}

	return moves
}

// GenerateRookMoves returns all pseudo-legal rook moves for the given color
// in the position
func GenerateRookMoves(pos Position, color Color) []Move {
	var moves []Move
	rooks := pos.Pieces[Rook] & pos.Colors[color]

	for rooks != 0 {
		from := rooks.PopLSB()
		attacks := rookAttacksMagic(from, pos.Pieces[AllPieces]) & ^pos.Colors[color]

		for attacks != 0 {
			to := attacks.PopLSB()
			m := NewMove(from, to)
			moves = append(moves, m)
		}
	}

	return moves
}

// GenerateBishopMoves returns all pseudo-legal bishop moves for the given color
// in the position
func GenerateBishopMoves(pos Position, color Color) []Move {
	var moves []Move
	bishops := pos.Pieces[Bishop] & pos.Colors[color]

	for bishops != 0 {
		from := bishops.PopLSB()
		attacks := bishopAttacksMagic(from, pos.Pieces[AllPieces]) & ^pos.Colors[color]

		for attacks != 0 {
			to := attacks.PopLSB()
			m := NewMove(from, to)
			moves = append(moves, m)
		}
	}

	return moves
}

// appendPawnMove appends the move from-to to moves, expanding it into the
// four promotion moves if to is on the last rank for the given color.
func appendPawnMove(moves []Move, from, to Square, color Color) []Move {
	promotionRank := 7
	if color == Black {
		promotionRank = 0
	}

	if to.Rank() != promotionRank {
		return append(moves, NewMove(from, to))
	}

	return append(moves,
		NewPromotionMove(from, to, Queen),
		NewPromotionMove(from, to, Rook),
		NewPromotionMove(from, to, Bishop),
		NewPromotionMove(from, to, Knight),
	)
}

// GeneratePawnMoves returns all pseudo-legal pawn pushes and captures
// for the given color in the position.
func GeneratePawnMoves(pos Position, color Color) []Move {
	var moves []Move

	pawnPushes := PawnPush(pos, color)
	doublePawnPushes := DoublePawnPush(pos, color)

	pawnCapturesLeft := PawnCapturesLeft(pos, color)
	pawnCapturesRight := PawnCapturesRight(pos, color)

	for pawnPushes != 0 {
		to := pawnPushes.PopLSB()
		var from Square

		if color == White {
			from = to - 8
		} else {
			from = to + 8
		}

		moves = appendPawnMove(moves, from, to, color)
	}

	for doublePawnPushes != 0 {
		to := doublePawnPushes.PopLSB()
		var from Square

		if color == White {
			from = to - 16
		} else {
			from = to + 16
		}

		m := NewMove(from, to)
		moves = append(moves, m)
	}

	for pawnCapturesLeft != 0 {
		to := pawnCapturesLeft.PopLSB()
		var from Square

		if color == White {
			from = to - 7
		} else {
			from = to + 9
		}

		moves = appendPawnMove(moves, from, to, color)
	}

	for pawnCapturesRight != 0 {
		to := pawnCapturesRight.PopLSB()
		var from Square

		if color == White {
			from = to - 9
		} else {
			from = to + 7
		}

		moves = appendPawnMove(moves, from, to, color)
	}

	return moves
}

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

// PawnPush returns the bitboard of single-step forward pushes for pawns of the given color.
func PawnPush(pos Position, color Color) Bitboard {
	var targetRank Bitboard
	if color == White {
		targetRank = (pos.Pieces[Pawn] & pos.Colors[color]) << 8
	} else {
		targetRank = (pos.Pieces[Pawn] & pos.Colors[color]) >> 8
	}
	return ^pos.Pieces[AllPieces] & targetRank
}

// DoublePawnPush returns the bitboard of two-step forward pushes for pawns
// of the given color still on their starting rank.
func DoublePawnPush(pos Position, color Color) Bitboard {
	var targetRank Bitboard
	if color == White {
		targetRank = (PawnPush(pos, color) & Rank3) << 8
	} else {
		targetRank = (PawnPush(pos, color) & Rank6) >> 8
	}
	return ^pos.Pieces[AllPieces] & targetRank
}

// PawnCapturesLeft returns the bitboard of capture-left diagonal squares
// for pawns of the given color that contain an enemy piece.
func PawnCapturesLeft(pos Position, color Color) Bitboard {
	pawns := pos.Pieces[Pawn] & pos.Colors[color]

	if color == White {
		return (pawns & ^FileA << 7) & pos.Colors[color^1]
	}
	return (pawns & ^FileA >> 9) & pos.Colors[color^1]
}

// PawnCapturesRight returns the bitboard of capture-right diagonal squares
// for pawns of the given color that contain an enemy piece.
func PawnCapturesRight(pos Position, color Color) Bitboard {
	pawns := pos.Pieces[Pawn] & pos.Colors[color]

	if color == White {
		return (pawns & ^FileH << 9) & pos.Colors[color^1]
	}
	return (pawns & ^FileH >> 7) & pos.Colors[color^1]
}

// PawnCaptures returns the bitboard of diagonal capture squares
// for pawns of the given color that contain an enemy piece.
func PawnCaptures(pos Position, color Color) Bitboard {
	return PawnCapturesLeft(pos, color) | PawnCapturesRight(pos, color)
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
