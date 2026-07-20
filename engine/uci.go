package engine

// String returns the UCI algebraic notation of the square (e.g. "e4").
func (s Square) String() string {
	file := "abcdefgh"[s.File()]
	rank := "12345678"[s.Rank()]
	return string(file) + string(rank)
}

// ParseSquare parses a square in UCI algebraic notation (e.g. "e4").
func ParseSquare(s string) Square {
	file := s[0] - 'a'
	rank := s[1] - '1'

	return Square(rank)*8 + Square(file)
}

func (m Move) UCI() string {
	from := m.From().String()
	to := m.To().String()
	pPiece := ""

	if m.IsPromotion() {
		switch m.Promotion() {
		case Queen:
			pPiece += "q"
		case Rook:
			pPiece += "r"
		case Knight:
			pPiece += "n"
		case Bishop:
			pPiece += "b"
		}
	}

	return from + to + pPiece
}

// ParseUCIMove parses a move in UCI long algebraic notation (e.g. "e2e4", "a7a8q")
func ParseUCIMove(s string) Move {
	var move Move

	from := ParseSquare(s[0:2])
	to := ParseSquare(s[2:4])

	if len(s) == 5 {
		pPiece := Queen
		switch s[4] {
		case 'r':
			pPiece = Rook
		case 'n':
			pPiece = Knight
		case 'b':
			pPiece = Bishop
		}

		move = NewPromotionMove(from, to, pPiece)
	} else {
		move = NewMove(from, to)
	}

	return move
}
