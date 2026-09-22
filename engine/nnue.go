package engine

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
)

// HalfKA feature layout, matching bullet's ChessBuckets input type with an
// identity king bucket (buckets[sq] = sq, one bucket per king square) — see
// crates/bullet_lib/src/game/inputs/{chess768,chess_buckets}.rs in the
// bullet checkout. A feature is (own king square, piece square, own/enemy,
// piece type) for every piece INCLUDING kings: unlike HalfKP (this file's
// previous scheme), the enemy king is a real feature too, not just the
// opponent's invisible anchor. Matching bullet's exact formula — down to
// its P/N/B/R/Q/K piece ordering, which differs from our own PieceType
// iota order — lets a bullet-trained weight file be loaded directly,
// without a translation layer.
const (
	nnueHidden     = 256
	nnuePieceKinds = 6                       // Pawn, Knight, Bishop, Rook, Queen, King (bullet's order)
	nnueColorBlock = 64 * nnuePieceKinds     // 384: one perspective's own or enemy pieces
	nnueFeatures   = 64 * 2 * nnueColorBlock // 64 king buckets * 768
)

// bulletPieceCode maps our PieceType (Pawn, Rook, Knight, Bishop, Queen,
// King — see types.go) to bullet's P/N/B/R/Q/K feature-table ordering.
var bulletPieceCode = [...]int{
	Pawn:   0,
	Knight: 1,
	Bishop: 2,
	Rook:   3,
	Queen:  4,
	King:   5,
}

// Layer sizes past the accumulator: the two perspectives are clipped and
// concatenated into one 2*nnueHidden input, then 32 -> 32 -> 1, the classic
// "HalfKP-256x2-32-32-1" shape.
const (
	nnueInputSize = 2 * nnueHidden
	nnueL1Size    = 32
	nnueL2Size    = 32

	// nnueClipMax is the ceiling clipped ReLU clamps activations to, so
	// they fit the quantized range the next layer's weights expect.
	nnueClipMax = 127
	// nnueWeightScale is the fixed-point scale stored weights are
	// expressed in (a stored value v represents the real-valued weight
	// v/nnueWeightScale); dividing it back out after each dot product
	// keeps the numbers in the same scale from layer to layer.
	nnueWeightScale = 64
)

// nnueFeatureWeights and nnueFeatureBias are placeholder weights: a small
// net has not been trained yet, so these are deterministically seeded
// random values (like the Zobrist keys in zobrist.go) purely so the
// accumulator equivalence tests exercise real arithmetic instead of
// trivially comparing all-zero accumulators. Replace with a loader for a
// trained file once one exists.
var (
	nnueFeatureWeights [nnueFeatures][nnueHidden]int16
	nnueFeatureBias    [nnueHidden]int16

	// Placeholder weights for the layers past the accumulator — random for
	// the same reason as nnueFeatureWeights above.
	nnueL1Weights  [nnueInputSize][nnueL1Size]int16
	nnueL1Bias     [nnueL1Size]int32
	nnueL2Weights  [nnueL1Size][nnueL2Size]int16
	nnueL2Bias     [nnueL2Size]int32
	nnueOutWeights [nnueL2Size]int16
	nnueOutBias    int32
)

func init() {
	rng := rand.New(rand.NewSource(2))
	for i := range nnueFeatureWeights {
		for j := range nnueFeatureWeights[i] {
			nnueFeatureWeights[i][j] = int16(rng.Intn(201) - 100)
		}
	}
	for j := range nnueFeatureBias {
		nnueFeatureBias[j] = int16(rng.Intn(201) - 100)
	}

	for i := range nnueL1Weights {
		for j := range nnueL1Weights[i] {
			nnueL1Weights[i][j] = int16(rng.Intn(255) - 127)
		}
	}
	for j := range nnueL1Bias {
		nnueL1Bias[j] = int32(rng.Intn(255) - 127)
	}
	for i := range nnueL2Weights {
		for j := range nnueL2Weights[i] {
			nnueL2Weights[i][j] = int16(rng.Intn(255) - 127)
		}
	}
	for j := range nnueL2Bias {
		nnueL2Bias[j] = int32(rng.Intn(255) - 127)
	}
	for i := range nnueOutWeights {
		nnueOutWeights[i] = int16(rng.Intn(255) - 127)
	}
	nnueOutBias = int32(rng.Intn(255) - 127)
}

// orientSquare maps a square to the same square as seen from perspective's
// side (mirrored vertically for Black), matching the ^56 convention used
// for PSTs in eval.go.
func orientSquare(perspective Color, s Square) Square {
	if perspective == Black {
		return s ^ 56
	}
	return s
}

// halfKAIndex returns the feature-table row for a piece of pieceColor on
// pieceSq (any type, including King), as seen from perspective (whose king
// stands on kingSq).
func halfKAIndex(perspective Color, kingSq, pieceSq Square, pt PieceType, pieceColor Color) int {
	relKing := orientSquare(perspective, kingSq)
	relSq := orientSquare(perspective, pieceSq)
	colorOffset := 0
	if pieceColor != perspective {
		colorOffset = nnueColorBlock
	}
	return int(relKing)*2*nnueColorBlock + colorOffset + 64*bulletPieceCode[pt] + int(relSq)
}

// nnueAccumulator holds, per perspective, the running sum of active
// HalfKA feature rows. dirty is set whenever that perspective's own king
// moves (its features' king anchor changed, invalidating every existing
// row) and cleared by the next refresh — see Accumulator.
type nnueAccumulator struct {
	values [2][nnueHidden]int16
	dirty  [2]bool
}

func addRow(dst *[nnueHidden]int16, row *[nnueHidden]int16) {
	for i := range dst {
		dst[i] += row[i]
	}
}

func subRow(dst *[nnueHidden]int16, row *[nnueHidden]int16) {
	for i := range dst {
		dst[i] -= row[i]
	}
}

// nnueAddPiece and nnueRemovePiece are called from PutPiece/RemovePiece for
// every piece placement, mirroring how those functions maintain hash
// incrementally.
//
// Unlike the old HalfKP scheme, the king is now a feature like any other
// piece, so a king move needs different treatment per perspective: for its
// OWN perspective it's also the anchor (own king square selects the whole
// 768-wide block, see halfKAIndex), so every existing row in that
// perspective's accumulator is stale — that perspective is flagged dirty
// rather than eagerly recomputed here, since refreshing now would be
// premature (placement order during position setup is unspecified, see
// StartPos/ParseFEN, and UnmakeMove restores the king before a captured
// piece when a king move was also a capture, position.go's UnmakeMove) —
// the lazy refresh on read (Accumulator) sidesteps both. For the OPPONENT's
// perspective, though, this king is an ordinary feature (their own anchor
// hasn't moved), so it gets an ordinary incremental update.
func (p *Position) nnueAddPiece(s Square, c Color, pt PieceType) {
	for persp := White; persp <= Black; persp++ {
		if pt == King && persp == c {
			p.acc.dirty[persp] = true
			continue
		}
		if p.acc.dirty[persp] || p.Pieces[King]&p.Colors[persp] == 0 {
			continue
		}
		idx := halfKAIndex(persp, p.KingSquare(persp), s, pt, c)
		addRow(&p.acc.values[persp], &nnueFeatureWeights[idx])
	}
}

func (p *Position) nnueRemovePiece(s Square, c Color, pt PieceType) {
	for persp := White; persp <= Black; persp++ {
		if pt == King && persp == c {
			p.acc.dirty[persp] = true
			continue
		}
		if p.acc.dirty[persp] || p.Pieces[King]&p.Colors[persp] == 0 {
			continue
		}
		idx := halfKAIndex(persp, p.KingSquare(persp), s, pt, c)
		subRow(&p.acc.values[persp], &nnueFeatureWeights[idx])
	}
}

// accumulatorFromScratch recomputes perspective's accumulator directly from
// the current bitboards, ignoring any incrementally maintained state. It is
// the independent reference used by refresh and by
// TestAccumulatorMatchesFromScratch, mirroring hashFromScratch in
// zobrist.go.
func (p *Position) accumulatorFromScratch(perspective Color) [nnueHidden]int16 {
	kingSq := p.KingSquare(perspective)
	acc := nnueFeatureBias
	for pt := Pawn; pt <= King; pt++ {
		for c := White; c <= Black; c++ {
			bb := p.Pieces[pt] & p.Colors[c]
			for bb != 0 {
				sq := bb.PopLSB()
				idx := halfKAIndex(perspective, kingSq, sq, pt, c)
				addRow(&acc, &nnueFeatureWeights[idx])
			}
		}
	}
	return acc
}

// Accumulator returns perspective's up-to-date HalfKP accumulator,
// refreshing it from scratch first if perspective's king has moved since
// the last refresh.
func (p *Position) Accumulator(perspective Color) *[nnueHidden]int16 {
	if p.acc.dirty[perspective] {
		p.acc.values[perspective] = p.accumulatorFromScratch(perspective)
		p.acc.dirty[perspective] = false
	}
	return &p.acc.values[perspective]
}

// clippedReLU clamps x to [0, nnueClipMax]: negative activations become 0,
// and anything above the ceiling is capped so it fits the range the next
// layer's quantized weights were scaled for.
func clippedReLU(x int32) int32 {
	if x < 0 {
		return 0
	}
	if x > nnueClipMax {
		return nnueClipMax
	}
	return x
}

// nnueForward runs the network past the accumulator (256x2 -> 32 -> 32 -> 1)
// and returns the raw output, still one factor of nnueClipMax away from
// fully dequantized (every layer here only divides by nnueWeightScale, not
// nnueClipMax*nnueWeightScale, to keep intermediate values in the same
// integer scale the next layer's weights expect) — see NNUEEvaluate, which
// finishes the conversion into centipawns. With the random placeholder
// weights this file starts with (see nnueFeatureWeights), the output is
// meaningless beyond exercising the arithmetic; call LoadNNUEWeights to get
// a trained network. Side to move's own accumulator goes first, then the
// opponent's, matching the usual NNUE convention so the same weights are
// meaningful regardless of which color is moving.
func (p *Position) nnueForward() int32 {
	own := p.Accumulator(p.SideToMove)
	opp := p.Accumulator(p.SideToMove ^ 1)

	var input [nnueInputSize]int32
	for i := 0; i < nnueHidden; i++ {
		input[i] = clippedReLU(int32(own[i]))
		input[nnueHidden+i] = clippedReLU(int32(opp[i]))
	}

	var l1out [nnueL1Size]int32
	for j := range l1out {
		sum := nnueL1Bias[j]
		for i, v := range input {
			sum += v * int32(nnueL1Weights[i][j])
		}
		l1out[j] = clippedReLU(sum / nnueWeightScale)
	}

	var l2out [nnueL2Size]int32
	for j := range l2out {
		sum := nnueL2Bias[j]
		for i, v := range l1out {
			sum += v * int32(nnueL2Weights[i][j])
		}
		l2out[j] = clippedReLU(sum / nnueWeightScale)
	}

	sum := nnueOutBias
	for i, v := range l2out {
		sum += v * int32(nnueOutWeights[i])
	}
	return sum / nnueWeightScale
}

// nnueEvalScale matches the `eval_scale` used when training (see
// ~/projects/bullet/examples/kissengine.rs): sigmoid(centipawns /
// nnueEvalScale) is the network's implied win probability. nnueForward's
// return value still carries one extra factor of nnueClipMax — every layer
// divides its raw sum by nnueWeightScale (QB) to cancel that layer's own
// weight scale, which also happens to leave every intermediate activation
// at nnueClipMax (QA) times its real value throughout, all the way to the
// final (unactivated) output — so turning nnueForward's return value into
// the real, unscaled output needs one more division by QA alone, not QA*QB.
const nnueEvalScale = 400

// useNNUE selects which static evaluation search.go's two call sites use.
// Off by default, so search behavior and every existing test are unchanged
// unless something explicitly opts in via SetUseNNUE — this is an A/B
// comparison switch for the NNUE experiment (see nnue.go/nnue_test.go), not
// a permanent feature: unconditionally swapping Evaluate for NNUEEvaluate
// everywhere would change search behavior (static null move margins,
// quiescence stand-pat, ...) throughout the test suite for a net that
// isn't trained to adoption quality yet.
var useNNUE = false

// SetUseNNUE flips the switch above. Not concurrency-safe against a running
// search, same as SetHashSize (tt.go) — call it only between searches.
func SetUseNNUE(enabled bool) {
	useNNUE = enabled
}

// staticEvalDispatch is what search.go actually calls: Evaluate(pos)
// normally, or NNUEEvaluate() when useNNUE is set. Both share the
// White-relative convention (see eval.go), so callers negate for Black to
// move exactly as they did when calling Evaluate directly. (Named to avoid
// colliding with eval.go's own unexported evaluate(pos, traces...).)
func staticEvalDispatch(pos *Position) int {
	if useNNUE {
		return int(pos.NNUEEvaluate())
	}
	return Evaluate(pos)
}

// NNUEEvaluate returns the network's evaluation in centipawns, from White's
// perspective — the same convention as Evaluate() in eval.go, so the two
// are interchangeable at call sites (callers negate for Black to move).
func (p *Position) NNUEEvaluate() int32 {
	score := p.nnueForward() * nnueEvalScale / nnueClipMax
	if p.SideToMove == Black {
		score = -score
	}
	return score
}

// LoadNNUEWeights loads a network trained by bullet (see
// ~/projects/bullet/examples/kissengine.rs), replacing the random
// placeholder weights. The file must be bullet's quantised.bin for exactly
// this architecture (HalfKA 49152 -> 256x2 -> 32 -> 32 -> 1, QA=127,
// QB=64): l0w, l0b, l1w, l1b, l2w, l2b, outw, outb, each little-endian,
// column-major, written back to back with no padding between sections (see
// docs/4-saved-networks.md in the bullet checkout) — the array shapes in
// this file were deliberately chosen ([feature][hidden] etc., i.e.
// input-major) to match that column-major layout directly, so every
// section below is a straight sequential read with no transposition.
// Biases are stored quantised to int16 in the file but kept as int32 here
// (see nnueL1Bias etc.), so those sections widen element by element.
func LoadNNUEWeights(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)

	readInto := func(name string, dst any) error {
		if err := binary.Read(r, binary.LittleEndian, dst); err != nil {
			return fmt.Errorf("reading %s: %w", name, err)
		}
		return nil
	}

	readBiasInto := func(name string, dst []int32) error {
		raw := make([]int16, len(dst))
		if err := readInto(name, raw); err != nil {
			return err
		}
		for i, v := range raw {
			dst[i] = int32(v)
		}
		return nil
	}

	if err := readInto("l0w", &nnueFeatureWeights); err != nil {
		return err
	}
	if err := readInto("l0b", &nnueFeatureBias); err != nil {
		return err
	}
	if err := readInto("l1w", &nnueL1Weights); err != nil {
		return err
	}
	if err := readBiasInto("l1b", nnueL1Bias[:]); err != nil {
		return err
	}
	if err := readInto("l2w", &nnueL2Weights); err != nil {
		return err
	}
	if err := readBiasInto("l2b", nnueL2Bias[:]); err != nil {
		return err
	}
	if err := readInto("outw", &nnueOutWeights); err != nil {
		return err
	}
	var outBias [1]int32
	if err := readBiasInto("outb", outBias[:]); err != nil {
		return err
	}
	nnueOutBias = outBias[0]

	return nil
}
