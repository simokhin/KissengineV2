package engine

import (
	"math"
	"math/bits"
)

const (
	Exact TTFlag = iota
	LowerBound
	UpperBound
)

// TTFlag indicated how a TTEntry's score relates to the true value
// of the position: an exact value, or a bound established via
// alpha-beta pruning.
type TTFlag uint8

// TTEntry is what ttProbe returns: the unpacked contents of a slot, a
// previously searched position's result for reuse when the same position is
// reached again via a different move order.
type TTEntry struct {
	hash     uint64
	depth    int
	score    int
	flag     TTFlag
	bestMove Move
}

// ttSlot is the packed form entries are stored in: 8+4+2+1+1 = 16 bytes with
// no padding, so four slots fill a cache line. Scores fit an int32 (mate is
// about 100000) and depth is clamped to an int8 when stored.
type ttSlot struct {
	key      uint64
	score    int32
	bestMove Move
	depth    int8
	flag     TTFlag
}

const (
	ttSlotBytes = 16
	// DefaultHashMB is the default table size (1<<24 slots).
	DefaultHashMB = 256
)

var (
	// tTable is the transposition table, indexed by a masked Zobrist hash.
	// Its length is a power of two, ttMask is length-1.
	tTable []ttSlot
	ttMask uint64
)

func init() {
	SetHashSize(DefaultHashMB)
}

// SetHashSize reallocates the table to the largest power-of-two slot count
// that fits in mb megabytes (at least 1MB); the new table is empty. It must
// not be called while a search is running.
func SetHashSize(mb int) {
	slots := uint64(max(mb, 1)) << 20 / ttSlotBytes
	slots = 1 << (bits.Len64(slots) - 1)

	tTable = make([]ttSlot, slots)
	ttMask = slots - 1
}

// ClearHash empties the table, for a new game.
func ClearHash() {
	clear(tTable)
}

func ttIndex(hash uint64) uint64 {
	return hash & ttMask
}

// ttStore writes an entry into the tTable, replacing the existing entry
// only if it's from a different position or the new result was computed
// at an equal or greater depth.
func ttStore(hash uint64, depth, score int, flag TTFlag, bestMove Move) {
	slot := &tTable[ttIndex(hash)]

	if slot.key != hash || depth >= int(slot.depth) {
		*slot = ttSlot{
			key:      hash,
			score:    int32(score),
			bestMove: bestMove,
			depth:    int8(min(depth, math.MaxInt8)),
			flag:     flag,
		}
	}
}

// ttProbe looks up hash in the tTable, returning the stored entry and true
// only if the slot actually holds that exact position
// (not just a hash-collision on the same slot).
func ttProbe(hash uint64) (TTEntry, bool) {
	slot := &tTable[ttIndex(hash)]

	if slot.key != hash {
		return TTEntry{}, false
	}

	return TTEntry{
		hash:     slot.key,
		depth:    int(slot.depth),
		score:    int(slot.score),
		flag:     slot.flag,
		bestMove: slot.bestMove,
	}, true
}
