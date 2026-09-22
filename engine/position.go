package engine

type Position struct {
	Pieces [7]Bitboard
	Colors [2]Bitboard

	SideToMove Color
	// EnPassant is the square a pawn can capture to via en passant, or
	// NoSquare if no en passant capture is currently available.
	EnPassant Square

	Castling CastlingRights

	// FiftyMovesRule counts half-moves since the last pawn move or capture;
	// reachng 100 (50 half moves) makes the position a forced draw.
	FiftyMovesRule int

	mailbox [64]uint8

	// hash is the position's Zobrist hash, maintained incrementally by
	// PutPiece/RemovePiece and by the side-to-move/castling/en-passant
	// updates in MakeMove/UnmakeMove/MakeNullMove/UnmakeNullMove, rather
	// than recomputed from scratch on every Hash() call.
	hash uint64

	// acc is the HalfKA NNUE accumulator, incrementally maintained by
	// PutPiece/RemovePiece; see nnue.go.
	acc nnueAccumulator
}

type UndoPosition struct {
	Castling       CastlingRights
	EnPassant      Square
	FiftyMovesRule int
	CapturedPiece  PieceType
	CapturedSquare Square
}

// PutPiece places a piece of the given color and type on square s.
func (p *Position) PutPiece(s Square, c Color, pt PieceType) {
	p.mailbox[s] = uint8(pt)
	p.Pieces[pt] |= s.BB()
	p.Colors[c] |= s.BB()
	p.Pieces[AllPieces] |= s.BB()
	p.hash ^= zobristPieces[c][pt][s]
	p.nnueAddPiece(s, c, pt)
}

// RemovePiece removes the piece of the given color and type from square s.
func (p *Position) RemovePiece(s Square, c Color, pt PieceType) {
	p.Pieces[pt] &= ^s.BB()
	p.Colors[c] &= ^s.BB()
	p.Pieces[AllPieces] &= ^s.BB()
	p.hash ^= zobristPieces[c][pt][s]
	p.nnueRemovePiece(s, c, pt)
}

// StartPos returns a new Position set up in the standard chess starting configuration.
func StartPos() *Position {
	position := Position{}

	position.EnPassant = NoSquare
	position.Castling = WhiteKingside | WhiteQueenside | BlackKingside | BlackQueenside
	for castle := range 4 {
		position.hash ^= zobristCastling[castle]
	}

	// White pawns
	for s := A2; s <= H2; s++ {
		position.PutPiece(s, White, Pawn)
	}

	// Black pawns
	for s := A7; s <= H7; s++ {
		position.PutPiece(s, Black, Pawn)
	}

	// Rooks
	position.PutPiece(H1, White, Rook)
	position.PutPiece(A1, White, Rook)
	position.PutPiece(A8, Black, Rook)
	position.PutPiece(H8, Black, Rook)

	// Knights
	position.PutPiece(B1, White, Knight)
	position.PutPiece(G1, White, Knight)
	position.PutPiece(B8, Black, Knight)
	position.PutPiece(G8, Black, Knight)

	// Bishops
	position.PutPiece(C1, White, Bishop)
	position.PutPiece(F1, White, Bishop)
	position.PutPiece(C8, Black, Bishop)
	position.PutPiece(F8, Black, Bishop)

	// Queen
	position.PutPiece(D1, White, Queen)
	position.PutPiece(D8, Black, Queen)

	// King
	position.PutPiece(E1, White, King)
	position.PutPiece(E8, Black, King)

	return &position
}

// PieceAt returns the piece type standing on square s, or AllPieces if the
// square is empty.
func (p *Position) PieceAt(s Square) PieceType {
	if p.Pieces[AllPieces]&s.BB() == 0 {
		return AllPieces
	}
	return PieceType(p.mailbox[s])
}

// MakeMove applies the move to the position, updating piece placement,
// handling captures and promotions, and switching the side to move.
func (p *Position) MakeMove(m Move) UndoPosition {
	var undo UndoPosition

	undo.Castling = p.Castling

	from, to := m.From(), m.To()
	color := p.SideToMove

	movingPiece := p.PieceAt(from)

	if movingPiece == King {
		fileDiff := to.File() - from.File()
		if fileDiff == 2 || fileDiff == -2 {
			var rookFrom, rookTo Square
			if fileDiff == 2 {
				rookFrom = to + 1
				rookTo = to - 1
			} else {
				rookFrom = to - 2
				rookTo = to + 1
			}

			p.RemovePiece(rookFrom, color, Rook)
			p.PutPiece(rookTo, color, Rook)
		}
		if color == White {
			p.Castling &^= WhiteKingside | WhiteQueenside
		} else {
			p.Castling &^= BlackKingside | BlackQueenside
		}
	}

	if from == A1 || to == A1 {
		p.Castling &^= WhiteQueenside
	}
	if from == H1 || to == H1 {
		p.Castling &^= WhiteKingside
	}
	if from == A8 || to == A8 {
		p.Castling &^= BlackQueenside
	}
	if from == H8 || to == H8 {
		p.Castling &^= BlackKingside
	}

	changedCastling := undo.Castling ^ p.Castling
	for castle := range 4 {
		if changedCastling&CastlingRights(1<<castle) != 0 {
			p.hash ^= zobristCastling[castle]
		}
	}

	undo.EnPassant = p.EnPassant
	if movingPiece == Pawn && (to-from == 16 || from-to == 16) {
		p.EnPassant = (from + to) / 2
	} else {
		p.EnPassant = NoSquare
	}
	if undo.EnPassant != NoSquare {
		p.hash ^= zobristEnPassantFile[undo.EnPassant.File()]
	}
	if p.EnPassant != NoSquare {
		p.hash ^= zobristEnPassantFile[p.EnPassant.File()]
	}

	capturedPiece := p.PieceAt(to)
	undo.CapturedPiece = capturedPiece
	undo.CapturedSquare = to

	// Change FiftyMovesRule count
	if movingPiece == Pawn || capturedPiece != AllPieces {
		undo.FiftyMovesRule = p.FiftyMovesRule
		p.FiftyMovesRule = 0
	} else {
		undo.FiftyMovesRule = p.FiftyMovesRule
		p.FiftyMovesRule += 1
	}

	if capturedPiece != AllPieces {
		p.RemovePiece(to, color^1, capturedPiece)
	}

	if movingPiece == Pawn && from.File() != to.File() && capturedPiece == AllPieces {
		capturedSquare := to - 8
		if color == Black {
			capturedSquare = to + 8
		}

		undo.CapturedPiece = Pawn
		undo.CapturedSquare = capturedSquare

		p.RemovePiece(capturedSquare, color^1, Pawn)
	}

	p.RemovePiece(from, color, movingPiece)

	if m.IsPromotion() {
		p.PutPiece(to, color, m.Promotion())
	} else {
		p.PutPiece(to, color, movingPiece)
	}

	p.hash ^= zobristSideToMove
	p.SideToMove ^= 1

	return undo
}

// IsAttacked reports whether square s is attacked by any piece
// of the given color.
func (p *Position) IsAttacked(s Square, byColor Color) bool {
	if PawnAttacks[byColor^1][s]&p.Pieces[Pawn]&p.Colors[byColor] != 0 {
		return true
	} else if KnightAttacks[s]&p.Pieces[Knight]&p.Colors[byColor] != 0 {
		return true
	} else if KingAttacks[s]&p.Pieces[King]&p.Colors[byColor] != 0 {
		return true
	}

	rookLikeAttackers := p.Pieces[Rook] | p.Pieces[Queen]
	rookLikeAttackers &= p.Colors[byColor]

	if rookAttacksMagic(s, p.Pieces[AllPieces])&rookLikeAttackers != 0 {
		return true
	}

	bishopLikeAttackers := p.Pieces[Bishop] | p.Pieces[Queen]
	bishopLikeAttackers &= p.Colors[byColor]

	return bishopAttacksMagic(s, p.Pieces[AllPieces])&bishopLikeAttackers != 0
}

func (p *Position) UnmakeMove(m Move, undo UndoPosition) {
	changedCastling := p.Castling ^ undo.Castling
	for castle := range 4 {
		if changedCastling&CastlingRights(1<<castle) != 0 {
			p.hash ^= zobristCastling[castle]
		}
	}
	p.Castling = undo.Castling

	if p.EnPassant != NoSquare {
		p.hash ^= zobristEnPassantFile[p.EnPassant.File()]
	}
	if undo.EnPassant != NoSquare {
		p.hash ^= zobristEnPassantFile[undo.EnPassant.File()]
	}
	p.EnPassant = undo.EnPassant

	p.FiftyMovesRule = undo.FiftyMovesRule

	p.hash ^= zobristSideToMove
	p.SideToMove ^= 1

	to := m.To()
	from := m.From()
	color := p.SideToMove

	piece := p.PieceAt(to)

	// Remove piece from square where it moved to
	p.RemovePiece(to, color, piece)

	if m.IsPromotion() {
		p.PutPiece(from, color, Pawn)
	} else {
		p.PutPiece(from, color, piece)
	}

	if undo.CapturedPiece != AllPieces {
		p.PutPiece(undo.CapturedSquare, color^1, undo.CapturedPiece)
	}

	if piece == King {
		fileDiff := to.File() - from.File()
		if fileDiff == 2 || fileDiff == -2 {
			var rookFrom, rookTo Square
			if fileDiff == 2 {
				rookFrom = to + 1
				rookTo = to - 1
			} else {
				rookFrom = to - 2
				rookTo = to + 1
			}

			p.RemovePiece(rookTo, color, Rook)
			p.PutPiece(rookFrom, color, Rook)
		}
	}
}

func (p *Position) KingSquare(color Color) Square {
	return (p.Pieces[King] & p.Colors[color]).LSB()
}
