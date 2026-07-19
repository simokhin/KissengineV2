package engine

import "testing"

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
