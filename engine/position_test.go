package engine

import "testing"

func TestEnPassant(t *testing.T) {
	pos := *StartPos()

	if pos.EnPassant != NoSquare {
		t.Fatalf("want %d, get %d", NoSquare, pos.EnPassant)
	}

	move := NewMove(E2, E4)
	pos.MakeMove(move)

	if pos.EnPassant != E3 {
		t.Fatalf("want %d, get %d", E3, pos.EnPassant)
	}

	move = NewMove(E7, E6)
	pos.MakeMove(move)

	if pos.EnPassant != NoSquare {
		t.Fatalf("want %d, get %d", NoSquare, pos.EnPassant)
	}

}

func TestIsAttacked(t *testing.T) {
	pos := Position{}
	pos.PutPiece(E4, White, Pawn)

	if !pos.IsAttacked(D5, White) {
		t.Fatalf("D5 should be attacked by white pawn on E4")
	}
	if !pos.IsAttacked(F5, White) {
		t.Fatalf("F5 should be attacked by white pawn on E4")
	}
	if pos.IsAttacked(E5, White) {
		t.Fatalf("E5 should NOT be attacked")
	}

	pos2 := Position{}
	pos2.PutPiece(E5, Black, Pawn)

	if !pos2.IsAttacked(D4, Black) {
		t.Fatalf("D4 should be attacked by black pawn on E5")
	}
	if !pos2.IsAttacked(F4, Black) {
		t.Fatalf("F4 should be attacked by black pawn on E5")
	}
}

func TestMakeMove(t *testing.T) {
	pos := StartPos()

	move := NewMove(E2, E4)
	pos.MakeMove(move)

	if pos.PieceAt(E2) != AllPieces || pos.PieceAt(E4) != Pawn {
		t.Fatalf("want E2 empty and E4 pawn; get E2=%d E4=%d", pos.PieceAt(E2), pos.PieceAt(E4))
	}

	if pos.SideToMove != Black {
		t.Fatalf("want Black, get %d", pos.SideToMove)
	}
}

func TestPieceAt(t *testing.T) {
	pos := StartPos()

	rook := pos.PieceAt(A1)
	if rook != Rook {
		t.Fatalf("want %d, get %d", Rook, rook)
	}

	king := pos.PieceAt(E1)
	if king != King {
		t.Fatalf("want %d, get %d", King, king)
	}

	empty := pos.PieceAt(E4)
	if empty != AllPieces {
		t.Fatalf("want %d, get %d", AllPieces, empty)
	}
}

func TestStartPos(t *testing.T) {
	position := StartPos()

	pieceCount := position.Pieces[AllPieces].PopCount()
	if pieceCount != 32 {
		t.Errorf("want 32 pieces; get %d", pieceCount)
	}
}
