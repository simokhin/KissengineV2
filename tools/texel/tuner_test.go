package main

import (
	"go/format"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"kissengine-bitboard/engine"
)

// randomPositions returns n positions taken from seeded random games.
func randomPositions(n int, seed int64) []*engine.Position {
	rng := rand.New(rand.NewSource(seed))

	var out []*engine.Position
	for len(out) < n {
		pos := engine.StartPos()
		for ply := range 120 {
			var moves engine.MoveList
			engine.GenerateLegalMoves(*pos, pos.SideToMove, &moves)
			if len(moves.Slice()) == 0 {
				break
			}
			pos.MakeMove(moves.Slice()[rng.Intn(len(moves.Slice()))])
			if ply >= 6 && rng.Intn(4) == 0 {
				cp := *pos
				out = append(out, &cp)
			}
		}
	}

	return out[:n]
}

// labeledByEngine returns a dataset of the positions whose results are the
// win probability the engine's own evaluation predicts at the given K, so the
// current weights are (nearly) a perfect fit and the loss floor is ~0.
func labeledByEngine(positions []*engine.Position, k float64) *dataset {
	d := newDataset()
	for _, pos := range positions {
		d.add(pos, float32(1/(1+math.Exp(-kScale(k)*float64(engine.Evaluate(pos))))))
	}
	return d
}

func weightsToX(w [][2]int) []float64 {
	x := make([]float64, 2*len(w))
	for i, v := range w {
		x[2*i], x[2*i+1] = float64(v[0]), float64(v[1])
	}
	return x
}

func TestParseEPDLine(t *testing.T) {
	for _, tc := range []struct {
		line string
		want float32
	}{
		{`r2qkr2/p1pp1ppp/1pn1pn2/2P5/3Pb3/2N1P3/PP3PPP/R1B1KB1R b KQq - c9 "0-1";`, 0},
		{`4Q3/8/8/8/6k1/4K2p/3N4/5q2 b - - c9 "1/2-1/2";`, 0.5},
		{`rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - c9 "1-0";`, 1},
	} {
		pos, got, err := parseEPDLine([]byte(tc.line))
		if err != nil || got != tc.want || pos == nil {
			t.Errorf("parseEPDLine(%q) = %v, %v, %v; want result %v", tc.line, pos, got, err, tc.want)
		}
	}

	for _, bad := range []string{``, `8/8/8/8/8/8/8/8 w - -`, `4k3/8/8/8/8/8/8/4K3 w - - c9 "2-0";`} {
		if _, _, err := parseEPDLine([]byte(bad)); err == nil {
			t.Errorf("parseEPDLine(%q) succeeded, want an error", bad)
		}
	}
}

// TestLossMatchesEngine checks that the tuner's integer arithmetic on traces
// gives the loss that real engine.Evaluate calls give, for random weights.
func TestLossMatchesEngine(t *testing.T) {
	saved := engine.Weights()
	defer engine.SetWeights(saved)

	rng := rand.New(rand.NewSource(3))
	w := make([][2]int, engine.NumWeights)
	for i := range w {
		w[i] = [2]int{rng.Intn(401) - 200, rng.Intn(401) - 200}
	}
	engine.SetWeights(w)

	positions := randomPositions(3000, 1)
	d := newDataset()
	var viaEngine float64
	scale := kScale(1.1)
	for i, pos := range positions {
		result := float32(i%3) / 2
		d.add(pos, result)
		diff := float64(result) - 1/(1+math.Exp(-scale*float64(engine.Evaluate(pos))))
		viaEngine += diff * diff
	}
	viaEngine /= float64(len(positions))

	if got := d.evalInt(w, scale); math.Abs(got-viaEngine) > 1e-9 {
		t.Fatalf("loss from traces = %.12f, from engine.Evaluate = %.12f", got, viaEngine)
	}
}

func TestGradientMatchesFiniteDifferences(t *testing.T) {
	d := labeledByEngine(randomPositions(1500, 2), 1.2)
	rng := rand.New(rand.NewSource(4))

	x := weightsToX(engine.Weights())
	for j := range x {
		x[j] += rng.NormFloat64() * 15
	}
	scale := kScale(1.2)

	grad := make([]float64, len(x))
	d.eval(x, scale, grad)

	checked := 0
	for range 400 {
		j := rng.Intn(len(x))
		if grad[j] == 0 {
			continue
		}

		const h = 1e-2
		orig := x[j]
		x[j] = orig + h
		up := d.eval(x, scale, nil)
		x[j] = orig - h
		down := d.eval(x, scale, nil)
		x[j] = orig

		numeric := (up - down) / (2 * h)
		if math.Abs(numeric-grad[j]) > 1e-4*math.Abs(grad[j])+1e-10 {
			t.Errorf("weight %d (%s): analytic gradient %g, finite difference %g", j/2, engine.WeightName(j/2), grad[j], numeric)
		}
		checked++
	}
	if checked < 50 {
		t.Fatalf("only %d nonzero gradients checked", checked)
	}
}

func TestFitKRecoversK(t *testing.T) {
	d := labeledByEngine(randomPositions(3000, 5), 1.37)

	if got := fitK(d, weightsToX(engine.Weights())); math.Abs(got-1.37) > 0.02 {
		t.Errorf("fitK = %.4f, want about 1.37", got)
	}
}

// TestOptimizeRecoversWeights perturbs the weights of a dataset labeled by the
// engine itself and checks that the optimizer brings the loss back down close
// to the floor, and that frozen weights stay put.
func TestOptimizeRecoversWeights(t *testing.T) {
	d := labeledByEngine(randomPositions(6000, 6), 1.0)
	scale := kScale(1.0)

	truth := weightsToX(engine.Weights())
	frozen := frozenWeights(d, engine.NumWeights, 30)

	rng := rand.New(rand.NewSource(7))
	x := append([]float64(nil), truth...)
	for j := range x {
		if !frozen[j] {
			x[j] += rng.NormFloat64() * 20
		}
	}

	before := d.eval(x, scale, nil)
	optimize(d, x, scale, frozen, options{epochs: 300, lr: 1})
	after := d.eval(x, scale, nil)

	if after > 0.05*before {
		t.Errorf("loss went from %.6f to %.6f, want it below %.6f", before, after, 0.05*before)
	}
	for j := range x {
		if frozen[j] && x[j] != truth[j] {
			t.Fatalf("frozen value %d moved from %v to %v", j, truth[j], x[j])
		}
	}
}

// TestFloatEvalMatchesIntEval ties the floating-point loss the optimizer
// minimizes to the integer one the engine plays with. With every weight a
// multiple of 24 the phase blend divides exactly, so the two must agree; the
// gradient test alone can't see a mistake in the forward formula because it
// differentiates the same function.
func TestFloatEvalMatchesIntEval(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	w := make([][2]int, engine.NumWeights)
	for i := range w {
		w[i] = [2]int{24 * (rng.Intn(17) - 8), 24 * (rng.Intn(17) - 8)}
	}

	d := newDataset()
	for i, pos := range randomPositions(2000, 9) {
		d.add(pos, float32(i%3)/2)
	}

	scale := kScale(0.9)
	if got, want := d.eval(weightsToX(w), scale, nil), d.evalInt(w, scale); math.Abs(got-want) > 1e-9 {
		t.Fatalf("eval = %.12f, evalInt = %.12f", got, want)
	}
}

// TestKScaleConvention pins the meaning of K to Texel's usual one: with K = 1 a
// score of +400 gives the win probability 1/(1+10^-1) = 10/11.
func TestKScaleConvention(t *testing.T) {
	if got := 1 / (1 + math.Exp(-kScale(1)*400)); math.Abs(got-10.0/11) > 1e-12 {
		t.Errorf("win probability at +400 with K=1 is %.12f, want %.12f", got, 10.0/11)
	}
}

// TestRegularizationPullsBackUnobservedWeights: a weight that no position
// counts has no loss gradient, so without the L2 pull it stays wherever it
// is (this is how the evaluation's do-nothing directions drift), and with it
// the weight returns to its anchor.
func TestRegularizationPullsBackUnobservedWeights(t *testing.T) {
	d := labeledByEngine(randomPositions(500, 10), 1.0)
	scale := kScale(1.0)

	// The blocked-passed-pawn weight for the 8th rank can never be counted.
	unused := 2*(engine.NumWeights-1) + 0
	for _, e := range d.entries {
		if int(e.Index) == engine.NumWeights-1 {
			t.Skip("weight unexpectedly counted")
		}
	}

	anchor := weightsToX(engine.Weights())
	for _, reg := range []float64{0, 1e-8} {
		x := append([]float64(nil), anchor...)
		x[unused] += 40

		optimize(d, x, scale, make([]bool, len(x)), options{epochs: 200, lr: 1, reg: reg, anchor: anchor})

		moved := math.Abs(x[unused] - anchor[unused])
		switch {
		case reg == 0 && moved != 40:
			t.Errorf("without regularization the unobserved weight moved from +40 to %+.3f", x[unused]-anchor[unused])
		case reg != 0 && moved > 5:
			t.Errorf("with regularization the unobserved weight is still %.3f away from its anchor", moved)
		}
	}
}

// TestMeasurePieceValues: with weights that are material only (no PST, no
// bonuses, mg == eg) removing a piece changes the evaluation by exactly that
// piece's material, whichever side owns it and however the phase changes.
func TestMeasurePieceValues(t *testing.T) {
	saved := engine.Weights()
	defer engine.SetWeights(saved)

	material := map[string]int{
		"material/pawn": 101, "material/rook": 502, "material/knight": 303,
		"material/bishop": 354, "material/queen": 905,
	}
	w := make([][2]int, engine.NumWeights)
	for i := range w {
		if v, ok := material[engine.WeightName(i)]; ok {
			w[i] = [2]int{v, v}
		}
	}
	engine.SetWeights(w)

	got := measurePieceValues(randomPositions(300, 11))
	for pt, want := range map[engine.PieceType]float64{engine.Pawn: 101, engine.Rook: 502, engine.Knight: 303, engine.Bishop: 354, engine.Queen: 905} {
		if got[pt] != want {
			t.Errorf("%s measured %.3f, want %.0f", engine.PieceName(int(pt)), got[pt], want)
		}
	}
}

// TestMeasurePieceValuesSeesPositionalTerms: a knight standing on a square with
// a bonus is worth more than its material, which is the point of measuring.
func TestMeasurePieceValuesSeesPositionalTerms(t *testing.T) {
	saved := engine.Weights()
	defer engine.SetWeights(saved)

	w := make([][2]int, engine.NumWeights)
	for i := range w {
		switch engine.WeightName(i) {
		case "material/knight":
			w[i] = [2]int{300, 300}
		case "pst/knight/e4":
			w[i] = [2]int{40, 40}
		}
	}
	engine.SetWeights(w)

	pos := engine.ParseFEN("4k3/8/8/8/4N3/8/8/4K3 w - - 0 1")
	if got := measurePieceValues([]*engine.Position{pos})[engine.Knight]; got != 340 {
		t.Errorf("knight on e4 measured %.0f, want 340 (300 material + 40 square bonus)", got)
	}
}

// TestExistingPieceValues: what writeParams writes, existingPieceValues reads
// back, and a missing or empty file reports "not there" so the tuner measures.
func TestExistingPieceValues(t *testing.T) {
	values := [7]int{engine.Pawn: 100, engine.Rook: 500, engine.Knight: 320, engine.Bishop: 330, engine.Queen: 900}
	path := filepath.Join(t.TempDir(), "eval_params.go")

	if err := writeParams(path, make([][2]int, engine.NumWeights), values, ""); err != nil {
		t.Fatal(err)
	}
	if got, ok := existingPieceValues(path); !ok || got != values {
		t.Errorf("existingPieceValues = %v, %v; want %v, true", got, ok, values)
	}

	if _, ok := existingPieceValues(filepath.Join(t.TempDir(), "missing.go")); ok {
		t.Error("existingPieceValues of a missing file reported ok")
	}
	empty := filepath.Join(t.TempDir(), "empty.go")
	os.WriteFile(empty, []byte("package engine\n"), 0o644)
	if _, ok := existingPieceValues(empty); ok {
		t.Error("existingPieceValues of a file without the block reported ok")
	}
}

func TestWriteParams(t *testing.T) {
	w := make([][2]int, engine.NumWeights)
	for i := range w {
		w[i] = [2]int{i, -i}
	}
	values := [7]int{engine.Pawn: 109, engine.Rook: 576, engine.Knight: 404, engine.Bishop: 436, engine.Queen: 1136}
	path := filepath.Join(t.TempDir(), "eval_params.go")

	if err := writeParams(path, w, values, "Tuned on 3 positions of x."); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if formatted, err := format.Source(out); err != nil || string(formatted) != string(out) {
		t.Errorf("output is not gofmt-stable (err %v)", err)
	}
	text := string(out)
	for _, want := range []string{
		"// Code generated by tools/texel; DO NOT EDIT.",
		"Pawn:   109,", "Queen:  1136,", "King:   0,",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	// gofmt aligns the values of the map, so match any run of spaces.
	entry := regexp.MustCompile(`"[a-z0-9/-]+":\s+\{-?\d+, -?\d+\},`)
	if n := len(entry.FindAllString(text, -1)); n != engine.NumWeights {
		t.Errorf("%d weight entries, want %d", n, engine.NumWeights)
	}
	if !regexp.MustCompile(`"pst/knight/e4":\s+\{`).MatchString(text) {
		t.Error("no entry for pst/knight/e4")
	}

	if got := existingHeader(path); got != "Tuned on 3 positions of x." {
		t.Errorf("existingHeader = %q", got)
	}
	if got := existingHeader(filepath.Join(t.TempDir(), "missing.go")); got != "" {
		t.Errorf("existingHeader of a missing file = %q, want empty", got)
	}
}
