package engine

import "testing"

func TestStartPos(t *testing.T) {
	position := StartPos()

	pieceCount := position.Pieces[AllPieces].PopCount()
	if pieceCount != 32 {
		t.Errorf("want 32 pieces; get %d", pieceCount)
	}
}
