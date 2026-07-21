package engine

import "testing"

func BenchmarkSearchDepth(b *testing.B) {
	pos := StartPos()
	history := []uint64{pos.Hash()}

	for b.Loop() {
		tTable = [1 << 24]TTEntry{}

		SearchDepth(*pos, 6, history)
	}
}
