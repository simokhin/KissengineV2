// Command texel tunes the evaluation weights with Texel's method: it finds the
// weights that make a logistic function of the evaluation best predict the
// results of the games that quiet positions came from, minimizing the mean
// squared error.
//
//	go run ./tools/texel -data data/quiet-labeled.epd
//
// The training data is an EPD file in the Zurichess format (data/ is
// gitignored); one line in ten is held out to check for overfitting. The
// tuner never plays or evaluates positions itself: it calls engine.Trace once
// per position and then works on the counts alone. The result is written to
// engine/eval_params.go, which the engine loads by weight name.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"time"

	"kissengine-bitboard/engine"
)

func main() {
	var (
		dataPath = flag.String("data", "data/quiet-labeled.epd", "labeled positions: Zurichess EPD (`c9 \"1-0\";`) or lichess-big3-resolved (`[1.0]`); several files may be given separated by commas and are pooled")
		outPath  = flag.String("out", "engine/eval_params.go", "generated Go file to write")
		epochs   = flag.Int("epochs", 1000, "full-batch Adam epochs")
		lr       = flag.Float64("lr", 3, "initial learning rate, in centipawns per step")
		reg      = flag.Float64("reg", 1e-9, "strength of the L2 pull of the weights towards their starting values")
		minCount = flag.Int("min-count", 100, "leave weights counted in fewer training positions than this at their starting value")
		limit    = flag.Int("limit", 0, "use only the first N lines of the file (0 = all)")
		dry      = flag.Bool("dry", false, "report the result but don't write -out")
		measure  = flag.Bool("measure-only", false, "don't tune: keep the current weights and only re-measure the piece values")
		pieces   = flag.Bool("measure-pieces", false, "also re-measure the piece values (SEE and move ordering) after tuning; by default the ones already in -out are kept, so a retune doesn't silently change the search")
	)
	flag.Parse()

	var err error
	if *measure {
		err = runMeasureOnly(*dataPath, *outPath, *limit, *dry)
	} else {
		err = run(*dataPath, *outPath, *epochs, *lr, *reg, *minCount, *limit, *dry, *pieces)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "texel:", err)
		os.Exit(1)
	}
}

func run(dataPath, outPath string, epochs int, lr, reg float64, minCount, limit int, dry, measurePieces bool) error {
	start := time.Now()

	train, test, lines, err := loadData(dataPath, limit)
	if err != nil {
		return err
	}
	fmt.Printf("loaded %d training and %d held-out positions in %v\n", train.len(), test.len(), time.Since(start).Round(time.Millisecond))

	// x holds every weight as x[2*i] = middlegame, x[2*i+1] = endgame.
	initial := engine.Weights()
	x := make([]float64, 2*len(initial))
	for i, w := range initial {
		x[2*i], x[2*i+1] = float64(w[0]), float64(w[1])
	}

	frozen := frozenWeights(train, len(initial), minCount)

	k := fitK(train, x)
	scale := kScale(k)
	startTrain, startTest := train.eval(x, scale, nil), test.eval(x, scale, nil)
	fmt.Printf("K = %.4f; starting loss: train %.6f, held-out %.6f\n", k, startTrain, startTest)

	fmt.Println("epoch     lr   train loss  held-out loss")
	optimize(train, x, scale, frozen, options{
		epochs: epochs, lr: lr, reg: reg, anchor: append([]float64(nil), x...), progressEvery: 25,
		progress: func(epoch int, rate, loss float64) {
			fmt.Printf("%5d  %5.3f  %.8f  %.8f\n", epoch, rate, loss, test.eval(x, scale, nil))
		},
	})
	fmt.Printf("K refitted to the tuned weights: %.4f (fitted to the starting ones: %.4f)\n", fitK(train, x), k)

	tuned := make([][2]int, len(initial))
	for i := range tuned {
		tuned[i] = [2]int{int(math.Round(x[2*i])), int(math.Round(x[2*i+1]))}
	}
	endTrain, endTest := train.evalInt(tuned, scale), test.evalInt(tuned, scale)
	fmt.Printf("after rounding to integers: train %.6f (was %.6f), held-out %.6f (was %.6f)\n", endTrain, startTrain, endTest, startTest)

	printLargestChanges(initial, tuned, 20)
	printTableDrift(initial, tuned, frozen)

	if err := verifyAgainstEngine(lines, tuned, scale); err != nil {
		return err
	}

	pieceValues, keep := existingPieceValues(outPath)
	if measurePieces || !keep {
		engine.SetWeights(tuned)
		if pieceValues, err = pieceValuesFromLines(lines); err != nil {
			return err
		}
		printPieceValues(pieceValues)
	} else {
		fmt.Print("piece values kept from ", outPath, ":")
		for pt := range 5 {
			fmt.Printf("  %s %d", pieceTypeNames[pt], pieceValues[pt])
		}
		fmt.Println()
	}

	if dry {
		fmt.Println("dry run: not writing", outPath)
		return nil
	}

	header := fmt.Sprintf("Tuned on %d positions of %s (%d held out): K = %.4f, held-out loss %.6f -> %.6f.",
		train.len()+test.len(), dataPath, test.len(), k, startTest, endTest)
	if err := writeParams(outPath, tuned, pieceValues, header); err != nil {
		return err
	}
	fmt.Println("wrote", outPath, "in", time.Since(start).Round(time.Second))

	return nil
}

// runMeasureOnly rewrites the piece values in the generated file from the
// engine's current weights and leaves the weights themselves alone, so the
// effect of new piece values can be tested on its own.
func runMeasureOnly(dataPath, outPath string, limit int, dry bool) error {
	lines, err := readLines(dataPath, limit)
	if err != nil {
		return err
	}

	pieceValues, err := pieceValuesFromLines(lines)
	if err != nil {
		return err
	}
	printPieceValues(pieceValues)

	if dry {
		fmt.Println("dry run: not writing", outPath)
		return nil
	}

	if err := writeParams(outPath, engine.Weights(), pieceValues, existingHeader(outPath)); err != nil {
		return err
	}
	fmt.Println("wrote", outPath)

	return nil
}

func printPieceValues(values [7]int) {
	fmt.Print("measured piece values:")
	for pt := range 5 {
		fmt.Printf("  %s %d", pieceTypeNames[pt], values[pt])
	}
	fmt.Println()
}

// frozenWeights marks which entries of x must not move: weights counted in
// fewer than minCount training positions (Adam moves every parameter by about
// the learning rate however little evidence there is, so those would just
// wander).
func frozenWeights(train *dataset, numWeights, minCount int) []bool {
	counts := make([]int, numWeights)
	for _, e := range train.entries {
		counts[e.Index]++
	}

	frozen := make([]bool, 2*numWeights)
	rare := 0
	for i, c := range counts {
		if c < minCount {
			frozen[2*i], frozen[2*i+1] = true, true
			rare++
		}
	}
	fmt.Printf("%d of %d weights are counted in fewer than %d training positions and stay at their starting value\n", rare, numWeights, minCount)

	return frozen
}

func printLargestChanges(initial, tuned [][2]int, n int) {
	type change struct {
		name string
		half string
		from int
		to   int
	}

	var changes []change
	for i := range initial {
		for h, half := range []string{"mg", "eg"} {
			if initial[i][h] != tuned[i][h] {
				changes = append(changes, change{engine.WeightName(i), half, initial[i][h], tuned[i][h]})
			}
		}
	}

	sort.SliceStable(changes, func(a, b int) bool {
		return abs(changes[a].to-changes[a].from) > abs(changes[b].to-changes[b].from)
	})

	fmt.Printf("%d of %d values changed; the largest changes:\n", len(changes), 2*len(initial))
	for _, c := range changes[:min(n, len(changes))] {
		fmt.Printf("  %-28s %s %5d -> %5d\n", c.name, c.half, c.from, c.to)
	}
}

// printTableDrift prints, for each piece's PST, the average change of its
// unfrozen squares. A large average means the table has moved along one of the
// evaluation's do-nothing directions (see options.reg) rather than in shape.
func printTableDrift(initial, tuned [][2]int, frozen []bool) {
	fmt.Print("average PST change (mg/eg):")
	for pt := range 6 {
		var sum [2]float64
		n := 0
		for sq := range 64 {
			i := engine.PSTIndex(pt, sq)
			if frozen[2*i] {
				continue
			}
			n++
			sum[0] += float64(tuned[i][0] - initial[i][0])
			sum[1] += float64(tuned[i][1] - initial[i][1])
		}
		fmt.Printf("  %s %+.1f/%+.1f", engine.PieceName(pt), sum[0]/float64(max(n, 1)), sum[1]/float64(max(n, 1)))
	}
	fmt.Println()
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// verifyAgainstEngine is the tuner's self-check: on a sample of the positions it
// loads the tuned weights into the engine and compares the loss computed from
// real engine.Evaluate calls with the loss computed from the traces. They must
// agree, otherwise the tuner is optimizing something other than what the engine
// plays with.
func verifyAgainstEngine(lines [][]byte, tuned [][2]int, scale float64) error {
	step := max(1, len(lines)/20000)

	sample := newDataset()
	var positions []*engine.Position
	for i := 0; i < len(lines); i += step {
		pos, result, err := parseLine(lines[i])
		if err != nil {
			return fmt.Errorf("line %d: %w", i+1, err)
		}
		sample.add(pos, result)
		positions = append(positions, pos)
	}

	engine.SetWeights(tuned)
	var loss float64
	for p, pos := range positions {
		diff := float64(sample.result[p]) - 1/(1+math.Exp(-scale*float64(engine.Evaluate(pos))))
		loss += diff * diff
	}
	loss /= float64(len(positions))

	fromTraces := sample.evalInt(tuned, scale)
	if math.Abs(loss-fromTraces) > 1e-9 {
		return fmt.Errorf("self-check failed: loss via engine.Evaluate is %.12f but via traces %.12f", loss, fromTraces)
	}
	fmt.Printf("self-check: loss on %d positions is %.9f via engine.Evaluate and via traces\n", len(positions), loss)

	return nil
}
