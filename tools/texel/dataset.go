package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"

	"kissengine-bitboard/engine"
)

// totalPhase is the game phase at which the endgame weights have no effect.
// It mirrors engine.totalPhase; the tests compare against engine.Evaluate, so
// a mismatch would show up there.
const totalPhase = 24

// dataset holds labeled positions in the form the tuner works on: for each
// position not the board but its engine.Trace, i.e. how many times each
// weight was counted, plus the game phase and the result. All positions'
// entries live in one flat slice; position p owns entries[start[p]:start[p+1]].
type dataset struct {
	entries []engine.TraceEntry
	start   []int32
	phase   []uint8
	result  []float32 // 1 = White won, 0.5 = draw, 0 = Black won
}

func newDataset() *dataset {
	return &dataset{start: []int32{0}}
}

func (d *dataset) len() int {
	return len(d.phase)
}

func (d *dataset) add(pos *engine.Position, result float32) {
	entries, phase := engine.Trace(pos)
	d.entries = append(d.entries, entries...)
	d.start = append(d.start, int32(len(d.entries)))
	d.phase = append(d.phase, uint8(phase))
	d.result = append(d.result, result)
}

func (d *dataset) appendDataset(o *dataset) {
	base := int32(len(d.entries))
	d.entries = append(d.entries, o.entries...)
	for _, s := range o.start[1:] {
		d.start = append(d.start, s+base)
	}
	d.phase = append(d.phase, o.phase...)
	d.result = append(d.result, o.result...)
}

// parseEPDLine parses a line of the Zurichess format,
// `<placement> <side> <castling> <ep> c9 "1-0";`, into the position and the
// result from White's point of view. The lines carry no move counters.
func parseEPDLine(line []byte) (*engine.Position, float32, error) {
	f := strings.Fields(string(line))
	if len(f) < 6 || f[4] != "c9" {
		return nil, 0, fmt.Errorf("want 4 FEN fields then c9 \"result\";, got %q", line)
	}

	var result float32
	switch strings.Trim(f[5], `";`) {
	case "1-0":
		result = 1
	case "0-1":
		result = 0
	case "1/2-1/2":
		result = 0.5
	default:
		return nil, 0, fmt.Errorf("unknown result %s", f[5])
	}

	return engine.ParseFEN(strings.Join(f[:4], " ") + " 0 1"), result, nil
}

// isTestPosition assigns about one line in ten to the held-out set, by a hash
// of its index so that the split doesn't depend on how the file is ordered.
func isTestPosition(i int) bool {
	z := uint64(i) + 0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	return z%10 == 0
}

// readEPDLines returns the non-empty lines of the file (the first limit of
// them if limit > 0).
func readEPDLines(path string, limit int) ([][]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lines [][]byte
	for _, l := range bytes.Split(raw, []byte{'\n'}) {
		if len(bytes.TrimSpace(l)) > 0 {
			lines = append(lines, l)
		}
	}
	if limit > 0 && len(lines) > limit {
		lines = lines[:limit]
	}

	return lines, nil
}

// loadEPD reads the file (the first limit lines if limit > 0) and returns the
// training and held-out datasets plus the raw lines, which the final check
// against engine.Evaluate reuses. Lines are processed in parallel but merged
// in file order, so the result doesn't depend on the number of CPUs.
func loadEPD(path string, limit int) (train, test *dataset, lines [][]byte, err error) {
	lines, err = readEPDLines(path, limit)
	if err != nil {
		return nil, nil, nil, err
	}

	workers := runtime.NumCPU()
	trains, tests := make([]*dataset, workers), make([]*dataset, workers)
	errs := make([]error, workers)

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr, te := newDataset(), newDataset()
			trains[w], tests[w] = tr, te
			for i := len(lines) * w / workers; i < len(lines)*(w+1)/workers; i++ {
				pos, result, perr := parseEPDLine(lines[i])
				if perr != nil {
					errs[w] = fmt.Errorf("line %d: %w", i+1, perr)
					return
				}
				if isTestPosition(i) {
					te.add(pos, result)
				} else {
					tr.add(pos, result)
				}
			}
		}()
	}
	wg.Wait()

	train, test = newDataset(), newDataset()
	for w := range workers {
		if errs[w] != nil {
			return nil, nil, nil, errs[w]
		}
		train.appendDataset(trains[w])
		test.appendDataset(tests[w])
	}

	return train, test, lines, nil
}

// kScale converts Texel's K (the score s maps to a win probability
// 1/(1+10^(-K*s/400))) into the factor in front of s in a natural-base sigmoid.
func kScale(k float64) float64 {
	return k * math.Ln10 / 400
}

// eval returns the mean squared error between the results and the predicted
// win probability, with the weights in x laid out as x[2*i] = mg and
// x[2*i+1] = eg of weight i. If grad is non-nil it must have len(x) and
// receives the gradient of that mean. The evaluation is linear in x, so the
// derivative of a position's score with respect to weight i is just its trace
// count times the phase share of the middlegame or endgame half.
func (d *dataset) eval(x []float64, scale float64, grad []float64) float64 {
	n := d.len()
	workers := runtime.NumCPU()
	losses := make([]float64, workers)
	grads := make([][]float64, workers)

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()

			var g []float64
			if grad != nil {
				g = make([]float64, len(x))
				grads[w] = g
			}

			var loss float64
			for p := n * w / workers; p < n*(w+1)/workers; p++ {
				entries := d.entries[d.start[p]:d.start[p+1]]
				phase := float64(d.phase[p])

				var mg, eg float64
				for _, e := range entries {
					c := float64(e.Count)
					mg += c * x[2*int(e.Index)]
					eg += c * x[2*int(e.Index)+1]
				}

				score := (mg*phase + eg*(totalPhase-phase)) / totalPhase
				s := 1 / (1 + math.Exp(-scale*score))
				diff := float64(d.result[p]) - s
				loss += diff * diff

				if g != nil {
					dScore := -2 * diff * s * (1 - s) * scale
					mgShare, egShare := dScore*phase/totalPhase, dScore*(totalPhase-phase)/totalPhase
					for _, e := range entries {
						c := float64(e.Count)
						g[2*int(e.Index)] += mgShare * c
						g[2*int(e.Index)+1] += egShare * c
					}
				}
			}
			losses[w] = loss
		}()
	}
	wg.Wait()

	var total float64
	for w := range workers {
		total += losses[w]
	}
	if grad != nil {
		clear(grad)
		for w := range workers {
			for j, v := range grads[w] {
				grad[j] += v
			}
		}
		for j := range grad {
			grad[j] /= float64(n)
		}
	}

	return total / float64(n)
}

// evalInt is eval for integer weights, computed exactly as engine.Evaluate
// does (integer sums, integer division by the phase blend), without
// gradients. It is what the engine will actually play with after rounding.
func (d *dataset) evalInt(w [][2]int, scale float64) float64 {
	var loss float64
	for p := range d.len() {
		var mg, eg int
		for _, e := range d.entries[d.start[p]:d.start[p+1]] {
			mg += int(e.Count) * w[e.Index][0]
			eg += int(e.Count) * w[e.Index][1]
		}

		phase := int(d.phase[p])
		score := (mg*phase + eg*(totalPhase-phase)) / totalPhase
		diff := float64(d.result[p]) - 1/(1+math.Exp(-scale*float64(score)))
		loss += diff * diff
	}

	return loss / float64(d.len())
}
