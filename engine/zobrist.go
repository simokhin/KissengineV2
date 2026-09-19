package engine

import "math/rand"

// zobristPieces holds random keys for each (color, piece type, square)
// combination, used to build a Zobrist hash of a position.
var zobristPieces [2][6][64]uint64

// zobristSideToMove is XORed into the hash when it's Black's turn to move.
var zobristSideToMove uint64

// zobristCastling holds random keys for each of the four castling rights.
var zobristCastling [4]uint64

// zobristEnPassantFile holds random keys for each file an en passant
// capture might be available on.
var zobristEnPassantFile [8]uint64

// Hash returns the position's Zobrist hash. It is maintained incrementally
// by PutPiece/RemovePiece (piece placement) and by the side-to-move/
// castling/en-passant updates in MakeMove, UnmakeMove, MakeNullMove and
// UnmakeNullMove, rather than recomputed on every call.
func (p *Position) Hash() uint64 {
	return p.hash
}

// hashFromScratch recomputes the Zobrist hash by scanning the whole
// position, independently of the incrementally-maintained p.hash field.
// It exists only so tests can check the incremental maintenance against an
// independent computation -- never call this on a search hot path.
func (p *Position) hashFromScratch() uint64 {
	var hash uint64

	for color := range 2 {
		for pt := range 6 {
			pieces := p.Pieces[pt] & p.Colors[color]
			for pieces != 0 {
				sq := pieces.PopLSB()
				hash ^= zobristPieces[color][pt][sq]
			}
		}
	}

	if p.SideToMove == Black {
		hash ^= zobristSideToMove
	}

	for castle := range 4 {
		if p.Castling&(1<<castle) != 0 {
			hash ^= zobristCastling[castle]
		}
	}

	if p.EnPassant != NoSquare {
		hash ^= zobristEnPassantFile[p.EnPassant.File()]
	}

	return hash
}

// init fills the Zobrist key tables with random values from a fixed seed,
// so the tables (and therefore search behavior at a given depth/position)
// are reproducible across runs. Go 1.20+ auto-seeds the math/rand package
// source randomly at startup, so calling the package-level rand.Uint64()
// here would give a different table -- and therefore different
// transposition-table collisions and slightly different search results --
// on every process run.
func init() {
	rng := rand.New(rand.NewSource(1))

	for color := range 2 {
		for pt := range 6 {
			for sq := range 64 {
				zobristPieces[color][pt][sq] = rng.Uint64()
			}
		}
	}

	zobristSideToMove = rng.Uint64()

	for castle := range 4 {
		zobristCastling[castle] = rng.Uint64()
	}

	for file := range 8 {
		zobristEnPassantFile[file] = rng.Uint64()
	}
}
