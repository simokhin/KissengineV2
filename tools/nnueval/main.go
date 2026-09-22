// Command nnueval reports a bullet-trained network's held-out loss, since
// bullet's own test_set/validation-loss tracking isn't implemented in the
// checked-out version (prints a warning and does nothing — see
// ~/projects/bullet/examples/kissengine.rs). It reproduces the exact loss
// bullet trains against (mean squared error of sigmoid(cp/scale) vs the
// white-relative game result) through engine.NNUEEvaluate itself, so this
// also doubles as a check that the Go inference path agrees with whatever
// bullet computed internally, not just a hyperparameter-tuning aid.
//
//	go run ./tools/nnueval -net ~/projects/bullet/checkpoints/kissengine-v2-50/quantised.bin
//
// Run it against several checkpoints (see save_rate in kissengine.rs, one
// per 5 superbatches) to see where held-out loss stops improving — that,
// not the training loss bullet prints, is what should decide how many
// superbatches are worth keeping.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"

	"kissengine-bitboard/engine"
)

func main() {
	net := flag.String("net", "", "path to a bullet quantised.bin checkpoint (required)")
	dataPath := flag.String("data", "data/nnue-val.txt", "held-out positions, '<FEN> | <score> | <result>' per line")
	evalScale := flag.Float64("scale", 400, "eval_scale used when training: sigmoid(cp/scale) is the predicted win probability")
	flag.Parse()

	if *net == "" {
		log.Fatal("-net is required")
	}
	if err := run(*net, *dataPath, *evalScale); err != nil {
		log.Fatal(err)
	}
}

func run(net, dataPath string, evalScale float64) error {
	if err := engine.LoadNNUEWeights(net); err != nil {
		return fmt.Errorf("loading %s: %w", net, err)
	}

	f, err := os.Open(dataPath)
	if err != nil {
		return err
	}
	defer f.Close()

	var sumSquaredError float64
	var n int

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			return fmt.Errorf("malformed line (want '<FEN> | <score> | <result>'): %q", line)
		}
		fen := padFEN(strings.TrimSpace(parts[0]))
		result, err := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		if err != nil {
			return fmt.Errorf("bad result in line %q: %w", line, err)
		}

		pos := engine.ParseFEN(fen)
		cp := float64(pos.NNUEEvaluate())
		pred := 1 / (1 + math.Exp(-cp/evalScale))

		diff := pred - result
		sumSquaredError += diff * diff
		n++
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no positions read from %s", dataPath)
	}

	fmt.Printf("%s\n%d positions, held-out loss (MSE) = %.6f\n", net, n, sumSquaredError/float64(n))
	return nil
}

// padFEN appends placeholder halfmove/fullmove counters to a 4-field FEN
// (board, side to move, castling, en passant — Zurichess EPD's format, see
// data/quiet-labeled.epd) since engine.ParseFEN indexes fields[4]
// unconditionally and would panic without them. bullet's own FEN parser
// only ever needed the first two fields, so this padding was never needed
// on that side of the pipeline.
func padFEN(fen string) string {
	if len(strings.Fields(fen)) >= 6 {
		return fen
	}
	return fen + " 0 1"
}
