package engine

import "testing"

func TestPerft(t *testing.T) {
	pos := StartPos()

	nodes := perft(*pos, 1)
	if nodes != 20 {
		t.Fatalf("want %d, get %d", 20, nodes)
	}

	nodes = perft(*pos, 2)
	if nodes != 400 {
		t.Fatalf("want %d, get %d", 400, nodes)
	}

	nodes = perft(*pos, 3)
	if nodes != 8902 {
		t.Fatalf("want %d, get %d", 8902, nodes)
	}

	nodes = perft(*pos, 4)
	if nodes != 197281 {
		t.Fatalf("want %d, get %d", 197281, nodes)
	}

	nodes = perft(*pos, 5)
	if nodes != 4865609 {
		t.Fatalf("want %d, get %d", 4865609, nodes)
	}
}
