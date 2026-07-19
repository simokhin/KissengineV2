package engine

import "testing"

func TestNewMove(t *testing.T) {
	move := NewMove(E2, E4)
	from := move.From()
	to := move.To()

	if from != E2 || to != E4 {
		t.Fatalf("want from = %d and to = %d; get from = %d and to = %d", E2, E4, from, to)
	}
}
