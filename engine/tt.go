package engine

const (
	Exact TTFlag = iota
	LowerBound
	UpperBound
)

// TTFlag indicated how a TTEntry's score relates to the true value
// of the position: an exact value, or a bound established via
// alpha-beta pruning.
type TTFlag int

// TTEntry caches a previously searched position's result for reuse when
// the same position is reached again via a different move order.
type TTEntry struct {
	hash     uint64
	depth    int
	score    int
	flag     TTFlag
	bestMove Move
}

// tTable is the transposition table, indexed by a masked Zobrist hash.
var tTable [1 << 24]TTEntry

func ttIndex(hash uint64) uint64 {
	return hash & (uint64(len(tTable)) - 1)
}

// ttStore writes an entry into the tTable, replacing the existing entry
// only if it's from a different position or the new result was computed
// at an equal or greater depth.
func ttStore(hash uint64, depth, score int, flag TTFlag, bestMove Move) {
	index := ttIndex(hash)

	existing := tTable[index]
	if existing.hash != hash || depth >= existing.depth {
		tTable[index] = TTEntry{hash, depth, score, flag, bestMove}
	}
}

// ttProbe looks up hash in the tTable, returning the stored entry and true
// only if the slot actually holds that exact position
// (not just a hash-collision on the same slot).
func ttProbe(hash uint64) (TTEntry, bool) {
	index := ttIndex(hash)

	entry := tTable[index]

	if entry.hash == hash {
		return entry, true
	} else {
		return TTEntry{}, false
	}
}
