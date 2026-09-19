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

// maxMoves bounds the fixed-size MoveList buffer. 218 is the documented
// maximum number of *legal* moves in any reachable chess position, but
// MoveList also holds pseudo-legal move lists (GenerateMoves), which can
// exceed the legal count since illegal moves haven't been filtered out yet
// -- so this leaves headroom above 218 rather than using it directly.
const maxMoves = 256

// MoveList is a fixed capacity move buffer.
type MoveList struct {
	moves [maxMoves]Move
	count int
}

func (l *MoveList) Add(m Move) {
	l.moves[l.count] = m
	l.count++
}

func (l *MoveList) Slice() []Move {
	return l.moves[:l.count]
}

func (l *MoveList) Reset() {
	l.count = 0
}

// GenerateLegalMoves returns all fully legal moves for the given color in the position,
// filtering out pseudo-legal moves that leave the moving side's king in check
func GenerateLegalMoves(pos Position, color Color, list *MoveList) {
	inCheck := pos.IsAttacked(pos.KingSquare(color), color^1)
	generateLegalMoves(pos, color, inCheck, list)
}

// generateLegalMoves is GenerateLegalMoves with inCheck precomputed by the
// caller. Negamax and Quiescence already know whether the side to move is
// in check before generating its moves; this lets them skip the redundant
// IsAttacked call GenerateLegalMoves would otherwise repeat.
func generateLegalMoves(pos Position, color Color, inCheck bool, list *MoveList) {
	var pseudo MoveList
	GenerateMoves(pos, color, &pseudo)

	pinned := pinnedPieces(&pos, color)

	list.Reset()
	for _, m := range pseudo.Slice() {
		piece := pos.PieceAt(m.From())
		isEnPassant := piece == Pawn && m.From().File() != m.To().File() && pos.PieceAt(m.To()) == AllPieces

		if !inCheck && piece != King && pinned&m.From().BB() == 0 && !isEnPassant {
			list.Add(m)
			continue
		}

		undo := pos.MakeMove(m)
		if !pos.IsAttacked(pos.KingSquare(color), color^1) {
			list.Add(m)
		}
		pos.UnmakeMove(m, undo)
	}
}

// GenerateLegalCaptures returns only the legal capture moves for color -
// cheaper than GenerateLegalMoves when quiet moves aren't needed (quiescence).
func GenerateLegalCaptures(pos Position, color Color, list *MoveList) {
	var pseudo MoveList
	GenerateMoves(pos, color, &pseudo)

	list.Reset()
	for _, m := range pseudo.Slice() {
		if !isCapture(pos, m) {
			continue
		}
		undo := pos.MakeMove(m)
		if !pos.IsAttacked(pos.KingSquare(color), color^1) {
			list.Add(m)
		}
		pos.UnmakeMove(m, undo)
	}
}

// GenerateMoves appends all pseudo-legal moves for the given color in the
// position to list. Does not reset list first, so callers that want only
// this call's moves must reset beforehand.
func GenerateMoves(pos Position, color Color, list *MoveList) {
	GeneratePawnMoves(pos, color, list)
	GenerateKingMoves(pos, color, list)
	GenerateKnightMoves(pos, color, list)
	GenerateBishopMoves(pos, color, list)
	GenerateRookMoves(pos, color, list)
	GenerateQueenMoves(pos, color, list)
	GenerateCastleMoves(pos, color, list)
}

// GenerateCastleMoves appends all pseudo-legal castling moves for the given color in the position to list.
func GenerateCastleMoves(pos Position, color Color, list *MoveList) {
	if color == White {
		if pos.Castling&WhiteKingside != 0 &&
			pos.PieceAt(F1) == AllPieces && pos.PieceAt(G1) == AllPieces &&
			!pos.IsAttacked(E1, Black) && !pos.IsAttacked(F1, Black) && !pos.IsAttacked(G1, Black) {
			list.Add(NewMove(E1, G1))
		}

		if pos.Castling&WhiteQueenside != 0 &&
			pos.PieceAt(D1) == AllPieces && pos.PieceAt(C1) == AllPieces && pos.PieceAt(B1) == AllPieces &&
			!pos.IsAttacked(E1, Black) && !pos.IsAttacked(D1, Black) && !pos.IsAttacked(C1, Black) {
			list.Add(NewMove(E1, C1))
		}
	} else {
		if pos.Castling&BlackKingside != 0 &&
			pos.PieceAt(F8) == AllPieces && pos.PieceAt(G8) == AllPieces &&
			!pos.IsAttacked(E8, White) && !pos.IsAttacked(F8, White) && !pos.IsAttacked(G8, White) {
			list.Add(NewMove(E8, G8))
		}

		if pos.Castling&BlackQueenside != 0 &&
			pos.PieceAt(D8) == AllPieces && pos.PieceAt(C8) == AllPieces && pos.PieceAt(B8) == AllPieces &&
			!pos.IsAttacked(E8, White) && !pos.IsAttacked(D8, White) && !pos.IsAttacked(C8, White) {
			list.Add(NewMove(E8, C8))
		}
	}
}

// GenerateQueenMoves appends all pseudo-legal queen moves for the given color
// in the position to list.
func GenerateQueenMoves(pos Position, color Color, list *MoveList) {
	queen := pos.Pieces[Queen] & pos.Colors[color]

	for queen != 0 {
		from := queen.PopLSB()
		attacks := queenAttacksMagic(from, pos.Pieces[AllPieces]) & ^pos.Colors[color]

		for attacks != 0 {
			to := attacks.PopLSB()
			list.Add(NewMove(from, to))
		}
	}
}

// GenerateRookMoves appends all pseudo-legal rook moves for the given color
// in the position to list.
func GenerateRookMoves(pos Position, color Color, list *MoveList) {
	rooks := pos.Pieces[Rook] & pos.Colors[color]

	for rooks != 0 {
		from := rooks.PopLSB()
		attacks := rookAttacksMagic(from, pos.Pieces[AllPieces]) & ^pos.Colors[color]

		for attacks != 0 {
			to := attacks.PopLSB()
			list.Add(NewMove(from, to))
		}
	}
}

// GenerateBishopMoves appends all pseudo-legal bishop moves for the given color
// in the position to list.
func GenerateBishopMoves(pos Position, color Color, list *MoveList) {
	bishops := pos.Pieces[Bishop] & pos.Colors[color]

	for bishops != 0 {
		from := bishops.PopLSB()
		attacks := bishopAttacksMagic(from, pos.Pieces[AllPieces]) & ^pos.Colors[color]

		for attacks != 0 {
			to := attacks.PopLSB()
			list.Add(NewMove(from, to))
		}
	}
}

// appendPawnMove adds the move from-to to list, expanding it into the
// four promotion moves if to is on the last rank for the given color.
func appendPawnMove(list *MoveList, from, to Square, color Color) {
	promotionRank := 7
	if color == Black {
		promotionRank = 0
	}

	if to.Rank() != promotionRank {
		list.Add(NewMove(from, to))
		return
	}

	list.Add(NewPromotionMove(from, to, Queen))
	list.Add(NewPromotionMove(from, to, Rook))
	list.Add(NewPromotionMove(from, to, Bishop))
	list.Add(NewPromotionMove(from, to, Knight))
}

// GeneratePawnMoves appends all pseudo-legal pawn pushes and captures
// for the given color in the position to list.
func GeneratePawnMoves(pos Position, color Color, list *MoveList) {
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

		appendPawnMove(list, from, to, color)
	}

	for doublePawnPushes != 0 {
		to := doublePawnPushes.PopLSB()
		var from Square

		if color == White {
			from = to - 16
		} else {
			from = to + 16
		}

		list.Add(NewMove(from, to))
	}

	for pawnCapturesLeft != 0 {
		to := pawnCapturesLeft.PopLSB()
		var from Square

		if color == White {
			from = to - 7
		} else {
			from = to + 9
		}

		appendPawnMove(list, from, to, color)
	}

	for pawnCapturesRight != 0 {
		to := pawnCapturesRight.PopLSB()
		var from Square

		if color == White {
			from = to - 9
		} else {
			from = to + 7
		}

		appendPawnMove(list, from, to, color)
	}
}

// GenerateKingMoves appends all pseudo-legal king moves for the given color in the position to list.
func GenerateKingMoves(pos Position, color Color, list *MoveList) {
	king := pos.Pieces[King] & pos.Colors[color]

	from := king.PopLSB()
	kingAttacks := KingAttacks[from] & ^pos.Colors[color]

	for kingAttacks != 0 {
		to := kingAttacks.PopLSB()
		list.Add(NewMove(from, to))
	}
}

// GenerateKnightMoves appends all pseudo-legal knight moves for the given color in the position to list.
func GenerateKnightMoves(pos Position, color Color, list *MoveList) {
	knights := pos.Pieces[Knight] & pos.Colors[color]

	for knights != 0 {
		from := knights.PopLSB()
		attacks := KnightAttacks[from] & ^pos.Colors[color]

		for attacks != 0 {
			to := attacks.PopLSB()
			list.Add(NewMove(from, to))
		}
	}
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
		return (pawns & ^FileA << 7) & (pos.Colors[color^1] | pos.EnPassant.BB())
	}
	return (pawns & ^FileA >> 9) & (pos.Colors[color^1] | pos.EnPassant.BB())
}

// PawnCapturesRight returns the bitboard of capture-right diagonal squares
// for pawns of the given color that contain an enemy piece.
func PawnCapturesRight(pos Position, color Color) Bitboard {
	pawns := pos.Pieces[Pawn] & pos.Colors[color]

	if color == White {
		return (pawns & ^FileH << 9) & (pos.Colors[color^1] | pos.EnPassant.BB())
	}
	return (pawns & ^FileH >> 7) & (pos.Colors[color^1] | pos.EnPassant.BB())
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
