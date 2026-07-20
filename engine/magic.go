package engine

import "math/rand"

// magicEntry holds the precomputed magic-hash lookup table for a slider
// piece on a single square.
type magicEntry struct {
	mask    Bitboard
	magic   uint64
	shift   uint
	attacks []Bitboard
}

// rookMagics holds the magic-hash lookup tables for rook attacks,
// indexed by square.
var rookMagics [64]magicEntry

// bishopMagics holds the magic-hash lookup tables for bishop attacks,
// indexed by square.
var bishopMagics [64]magicEntry

func subsets(mask Bitboard) []Bitboard {
	var result []Bitboard

	subset := Bitboard(0)
	for {
		result = append(result, subset)

		subset = (subset - mask) & mask
		if subset == 0 {
			break
		}
	}

	return result
}

func queenAttacksMagic(s Square, occupied Bitboard) Bitboard {
	return rookAttacksMagic(s, occupied) | bishopAttacksMagic(s, occupied)
}

// rookAttacksMagic returns the squares a rook on s attacks
// given the occupied squares, using the precomputed magic table.
func rookAttacksMagic(s Square, occupied Bitboard) Bitboard {
	e := rookMagics[s]
	index := (uint64(occupied&e.mask) * e.magic) >> uint64(e.shift)
	return e.attacks[index]
}

// bishopAttacksMagic returns the squares a bishop on s attacks
// given the occupied squares, using the precomputed magic table.
func bishopAttacksMagic(s Square, occupied Bitboard) Bitboard {
	e := bishopMagics[s]
	index := (uint64(occupied&e.mask) * e.magic) >> uint64(e.shift)
	return e.attacks[index]
}

// buildMagicEntry builds the attack lookup table for square s using the
// given precomputed magic number.
func buildMagicEntry(s Square, mask Bitboard, magic uint64, sliderAttacks func(Square, Bitboard) Bitboard) magicEntry {
	bits := mask.PopCount()
	shift := uint(64 - bits)

	attacks := make([]Bitboard, 1<<bits)
	for _, occ := range subsets(mask) {
		index := (uint64(occ) * magic) >> shift
		attacks[index] = sliderAttacks(s, occ)
	}

	return magicEntry{mask: mask, magic: magic, shift: shift, attacks: attacks}
}

// findMagic searches for a magic number for square s that maps every
// occupancy in mask to a unique index, using sliderAttacks (rookAttacks or
// bishopAttacks) to compute the real attack for each occupancy.
func findMagic(s Square, mask Bitboard, sliderAttacks func(Square, Bitboard) Bitboard) uint64 {
	occupancies := subsets(mask)
	bits := mask.PopCount()

	for {
		magic := rand.Uint64() & rand.Uint64() & rand.Uint64()

		table := make([]Bitboard, 1<<bits)
		used := make([]bool, 1<<bits)
		valid := true

		for _, occ := range occupancies {
			index := (uint64(occ) * magic) >> (64 - bits)
			attack := sliderAttacks(s, occ)

			if used[index] && table[index] != attack {
				valid = false
				break
			}

			used[index] = true
			table[index] = attack
		}

		if valid {
			return magic
		}
	}
}

// init builds the attack lookup tables for all 64 squares using the
// precomputed magic numbers.
func init() {
	initRankFileMasks()

	for s := A1; s <= H8; s++ {
		rookMagics[s] = buildMagicEntry(s, rookRelevantMask(s), RookMagicNumbers[s], rookAttacks)
		bishopMagics[s] = buildMagicEntry(s, bishopRelevantMask(s), BishopMagicNumbers[s], bishopAttacks)
	}
}
