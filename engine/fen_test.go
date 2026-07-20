package engine

import "testing"

func TestParseFENStartPos(t *testing.T) {
	want := StartPos()
	got := ParseFEN(StartFEN)

	if *want != *got {
		t.Fatalf("ParseFEN(StartFEN) = %+v, want %+v", got, want)
	}
}

func TestParseFENKiwipete(t *testing.T) {
	fen := "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"
	pos := ParseFEN(fen)

	if pos.SideToMove != White {
		t.Errorf("SideToMove = %v, want White", pos.SideToMove)
	}
	if pos.Castling != WhiteKingside|WhiteQueenside|BlackKingside|BlackQueenside {
		t.Errorf("Castling = %v, want all rights", pos.Castling)
	}
	if pos.EnPassant != NoSquare {
		t.Errorf("EnPassant = %v, want NoSquare", pos.EnPassant)
	}
	if pos.FiftyMovesRule != 0 {
		t.Errorf("FiftyMovesRule = %v, want 0", pos.FiftyMovesRule)
	}
	if pos.PieceAt(A8) != Rook || pos.Colors[Black]&A8.BB() == 0 {
		t.Errorf("A8 should hold a black rook")
	}
	if pos.PieceAt(E5) != Knight || pos.Colors[White]&E5.BB() == 0 {
		t.Errorf("E5 should hold a white knight")
	}
}

func TestParseFENEnPassant(t *testing.T) {
	fen := "rnbqkbnr/ppp1pppp/8/3pP3/8/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 3"
	pos := ParseFEN(fen)

	if pos.EnPassant != D6 {
		t.Errorf("EnPassant = %v, want D6", pos.EnPassant)
	}
}
