package engine

import "testing"

func TestNewPromotionMove(t *testing.T) {
	move := NewPromotionMove(A7, A8, Queen)

	isPromotion := move.IsPromotion()
	if !isPromotion {
		t.Fatal("want true; get false")
	}

	pPiece := move.Promotion()
	if pPiece != Queen {
		t.Fatalf("want %d; get %d", Queen, pPiece)
	}

	from := move.From()
	to := move.To()
	if from != A7 || to != A8 {
		t.Fatalf("want from = %d and to = %d; get from = %d and to = %d", A7, A8, from, to)
	}
}

func TestGeneratePromotionMove(t *testing.T) {
	pos := Position{}
	pos.PutPiece(A7, White, Pawn)

	var list MoveList
	GeneratePawnMoves(pos, White, &list)
	if list.count != 4 {
		t.Fatalf("want 4, get %d", list.count)
	}

}

func TestNewMove(t *testing.T) {
	move := NewMove(E2, E4)
	from := move.From()
	to := move.To()

	if from != E2 || to != E4 {
		t.Fatalf("want from = %d and to = %d; get from = %d and to = %d", E2, E4, from, to)
	}

	if move.IsPromotion() {
		t.Fatalf("want false; get true")
	}
}
