package engine

type Position struct {
	Pieces     [7]Bitboard
	Colors     [2]Bitboard
	SideToMove Color
	// EnPassant is the square a pawn can capture to via en passant, or
	// NoSquare if no en passant capture is currently available.
	EnPassant Square
	Castling  CastlingRights
}

// PutPiece places a piece of the given color and type on square s.
func (p *Position) PutPiece(s Square, c Color, pt PieceType) {
	p.Pieces[pt] |= s.BB()
	p.Colors[c] |= s.BB()
	p.Pieces[AllPieces] |= s.BB()
}

// StartPos returns a new Position set up in the standard chess starting configuration.
func StartPos() *Position {
	position := Position{}

	position.EnPassant = NoSquare
	position.Castling = WhiteKingside | WhiteQueenside | BlackKingside | BlackQueenside

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
	for pt := Pawn; pt <= King; pt++ {
		if p.Pieces[pt]&s.BB() != 0 {
			return pt
		}
	}

	return AllPieces
}

// MakeMove applies the move to the position, updating piece placement,
// handling captures and promotions, and switching the side to move.
func (p *Position) MakeMove(m Move) {
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

			p.Pieces[Rook] &= ^rookFrom.BB()
			p.Colors[color] &= ^rookFrom.BB()
			p.Pieces[Rook] |= rookTo.BB()
			p.Colors[color] |= rookTo.BB()
			p.Pieces[AllPieces] &= ^rookFrom.BB()
			p.Pieces[AllPieces] |= rookTo.BB()
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

	if movingPiece == Pawn && (to-from == 16 || from-to == 16) {
		p.EnPassant = (from + to) / 2
	} else {
		p.EnPassant = NoSquare
	}

	capturedPiece := p.PieceAt(to)

	if capturedPiece != AllPieces {
		p.Pieces[capturedPiece] &= ^to.BB()
		p.Colors[color^1] &= ^to.BB()
	}

	if movingPiece == Pawn && from.File() != to.File() && capturedPiece == AllPieces {
		capturedSquare := to - 8
		if color == Black {
			capturedSquare = to + 8
		}

		p.Pieces[Pawn] &= ^capturedSquare.BB()
		p.Colors[color^1] &= ^capturedSquare.BB()
		p.Pieces[AllPieces] &= ^capturedSquare.BB()
	}

	p.Pieces[movingPiece] &= ^from.BB()

	if m.IsPromotion() {
		p.PutPiece(to, color, m.Promotion())
	} else {
		p.PutPiece(to, color, movingPiece)
	}

	p.Colors[color] ^= from.BB()
	p.Colors[color] |= to.BB()
	p.Pieces[AllPieces] ^= from.BB()
	p.Pieces[AllPieces] |= to.BB()

	p.SideToMove ^= 1
}

// IsAttacked reports whether square s is attacked by any piece
// of the given color.
func (p *Position) IsAttacked(s Square, byColor Color) bool {
	if KnightAttacks[s]&p.Pieces[Knight]&p.Colors[byColor] != 0 {
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

	if bishopAttacksMagic(s, p.Pieces[AllPieces])&bishopLikeAttackers != 0 {
		return true
	}

	pawnRank := s.Rank() - 1
	if byColor == Black {
		pawnRank = s.Rank() + 1
	}

	for _, df := range []int{-1, 1} {
		file := s.File() + df
		if onBoard(file, pawnRank) {
			sq := Square(pawnRank*8 + file)
			if p.Pieces[Pawn]&p.Colors[byColor]&sq.BB() != 0 {
				return true
			}
		}
	}

	return false
}

func (p *Position) KingSquare(color Color) Square {
	return (p.Pieces[King] & p.Colors[color]).LSB()
}
