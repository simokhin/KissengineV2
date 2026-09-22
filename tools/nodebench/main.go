// Command nodebench compares fixed-depth node counts between two engine
// builds — the working tree ("new") and a git ref ("base", default HEAD) —
// over a sample of positions. Fixed-depth node count is the cheap,
// deterministic way to judge a move-ordering or pruning change (search is
// deterministic at a fixed depth, see CLAUDE.md), but is chaotic on any one
// position, so this reports the geometric mean of the node ratio over many
// and how many positions improved/regressed rather than a single number.
//
//	go run ./tools/nodebench
//	go run ./tools/nodebench -depth 10 -n 40 7e90510
//
// Positions are sampled evenly across -positions (an EPD/FEN-per-line file,
// default testdata/openings-8ply.epd) so a small -n still spans the whole
// file instead of clustering near the front. A changed score or best move
// between base and new is flagged but is not necessarily a bug — pruning and
// ordering changes are expected to occasionally do that — it's there to be
// eyeballed. Both sides build with -pgo=off, like tools/match.sh, so the
// comparison is of code, not of whichever side happens to have a
// cmd/default.pgo lying around.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	var (
		depth     = flag.Int("depth", 9, "fixed search depth")
		posPath   = flag.String("positions", "testdata/openings-8ply.epd", "EPD/FEN-per-line file to sample positions from")
		n         = flag.Int("n", 40, "number of positions to sample, evenly spaced across the file")
		hashMB    = flag.Int("hash", 16, "UCI Hash size (MB) for each engine; small on purpose, cleared between every position")
		perPosMax = flag.Duration("timeout", 30*time.Second, "abort if one search doesn't produce a bestmove within this")
		outDir    = flag.String("out", "", "directory to build the two binaries into (default: bin/nodebench-<timestamp>, kept for inspection)")
	)
	flag.Parse()

	base := "HEAD"
	if flag.NArg() > 0 {
		base = flag.Arg(0)
	}

	if err := run(base, *depth, *posPath, *n, *hashMB, *perPosMax, *outDir); err != nil {
		fmt.Fprintln(os.Stderr, "nodebench:", err)
		os.Exit(1)
	}
}

func run(base string, depth int, posPath string, n, hashMB int, timeout time.Duration, outDir string) error {
	positions, err := samplePositions(posPath, n)
	if err != nil {
		return fmt.Errorf("reading positions: %w", err)
	}

	if outDir == "" {
		outDir = filepath.Join("bin", "nodebench-"+time.Now().Format("20060102150405"))
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	baseBin := filepath.Join(outDir, "base")
	newBin := filepath.Join(outDir, "new")
	fmt.Printf("building base=%s -> %s\n", base, baseBin)
	if err := buildRef(base, outDir, baseBin); err != nil {
		return fmt.Errorf("building base: %w", err)
	}
	fmt.Printf("building working tree -> %s\n", newBin)
	if err := buildWorkingTree(newBin); err != nil {
		return fmt.Errorf("building new: %w", err)
	}

	baseEng, err := startEngine(baseBin, hashMB)
	if err != nil {
		return fmt.Errorf("starting base engine: %w", err)
	}
	defer baseEng.close()

	newEng, err := startEngine(newBin, hashMB)
	if err != nil {
		return fmt.Errorf("starting new engine: %w", err)
	}
	defer newEng.close()

	fmt.Printf("\n%-4s %12s %12s %8s  %s\n", "#", "base-nodes", "new-nodes", "ratio", "flags")

	var logRatios []float64
	improved, regressed, mismatches := 0, 0, 0

	for i, fen := range positions {
		br, err := baseEng.searchFixed(fen, depth, timeout)
		if err != nil {
			return fmt.Errorf("base search on %q: %w", fen, err)
		}
		nr, err := newEng.searchFixed(fen, depth, timeout)
		if err != nil {
			return fmt.Errorf("new search on %q: %w", fen, err)
		}

		ratio := float64(nr.nodes) / float64(br.nodes)
		logRatios = append(logRatios, math.Log(ratio))
		switch {
		case ratio < 0.999:
			improved++
		case ratio > 1.001:
			regressed++
		}

		var flags []string
		if br.move != nr.move {
			flags = append(flags, fmt.Sprintf("MOVE %s->%s", br.move, nr.move))
			mismatches++
		}
		if br.score != nr.score {
			flags = append(flags, fmt.Sprintf("SCORE %s->%s", br.score, nr.score))
		}

		fmt.Printf("%-4d %12d %12d %8.3f  %s\n", i, br.nodes, nr.nodes, ratio, strings.Join(flags, " "))
	}

	geomean := math.Exp(mean(logRatios))
	fmt.Printf("\n%d positions: geomean node ratio (new/base) %.3f, %d improved, %d regressed, %d unchanged, %d best-move mismatches\n",
		len(positions), geomean, improved, regressed, len(positions)-improved-regressed, mismatches)

	return nil
}

func mean(xs []float64) float64 {
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

// samplePositions reads FEN lines from path and picks n of them, evenly
// spaced by index, so a small n still spans the whole file.
func samplePositions(path string, n int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("%s has no positions", path)
	}
	if n > len(lines) {
		n = len(lines)
	}

	sample := make([]string, n)
	for i := range sample {
		sample[i] = lines[i*len(lines)/n]
	}
	return sample, nil
}

// buildRef checks out ref into outDir/<ref-slug>-src via git archive and
// builds ./cmd from there. -pgo=off matches tools/match.sh: a cmd/default.pgo
// present only in the working tree must not give "new" an unrelated speed edge.
func buildRef(ref, outDir, binPath string) error {
	srcDir := filepath.Join(outDir, "base-src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return err
	}

	archive := exec.Command("git", "archive", ref)
	tar := exec.Command("tar", "-x", "-C", srcDir)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return err
	}
	tar.Stdin = pipe
	tar.Stderr = os.Stderr
	archive.Stderr = os.Stderr

	if err := tar.Start(); err != nil {
		return err
	}
	if err := archive.Run(); err != nil {
		return fmt.Errorf("git archive %s: %w", ref, err)
	}
	if err := tar.Wait(); err != nil {
		return fmt.Errorf("tar -x: %w", err)
	}

	build := exec.Command("go", "build", "-pgo=off", "-o", filepath.Join("..", "..", binPath), "./cmd")
	build.Dir = srcDir
	build.Stderr = os.Stderr
	build.Stdout = os.Stderr
	// binPath is relative to the repo root (outDir is under it); build.Dir is
	// outDir/base-src, two levels down, so walk back up the same distance.
	rel, err := filepath.Rel(srcDir, binPath)
	if err != nil {
		return err
	}
	build.Args[len(build.Args)-2] = rel
	return build.Run()
}

func buildWorkingTree(binPath string) error {
	abs, err := filepath.Abs(binPath)
	if err != nil {
		return err
	}
	build := exec.Command("go", "build", "-pgo=off", "-o", abs, "./cmd")
	build.Stderr = os.Stderr
	build.Stdout = os.Stderr
	return build.Run()
}

// engine is one running UCI engine process, fed one position at a time.
// ucinewgame is sent before every position: tTable is a package-level global
// that persists across searches, so without it a later position would run
// against a warm table seeded by earlier ones and the node counts would stop
// being comparable (see the tTable note in CLAUDE.md).
type engine struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan string
}

type searchResult struct {
	nodes int
	score string
	move  string
}

func startEngine(path string, hashMB int) (*engine, error) {
	cmd := exec.Command(path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	e := &engine{cmd: cmd, stdin: stdin, lines: make(chan string, 64)}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			e.lines <- sc.Text()
		}
		close(e.lines)
	}()

	fmt.Fprintln(e.stdin, "uci")
	if err := e.waitFor("uciok", 10*time.Second); err != nil {
		return nil, err
	}
	fmt.Fprintf(e.stdin, "setoption name Hash value %d\n", hashMB)
	fmt.Fprintln(e.stdin, "isready")
	if err := e.waitFor("readyok", 10*time.Second); err != nil {
		return nil, err
	}

	return e, nil
}

func (e *engine) waitFor(prefix string, timeout time.Duration) error {
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-e.lines:
			if !ok {
				return fmt.Errorf("engine exited before %q", prefix)
			}
			if strings.HasPrefix(line, prefix) {
				return nil
			}
		case <-deadline:
			return fmt.Errorf("timed out waiting for %q", prefix)
		}
	}
}

func (e *engine) searchFixed(fen string, depth int, timeout time.Duration) (searchResult, error) {
	fmt.Fprintln(e.stdin, "ucinewgame")
	fmt.Fprintf(e.stdin, "position fen %s\n", fen)
	fmt.Fprintf(e.stdin, "go depth %d\n", depth)

	var lastInfo string
	deadline := time.After(timeout)
	for {
		select {
		case line, ok := <-e.lines:
			if !ok {
				return searchResult{}, fmt.Errorf("engine exited mid-search")
			}
			if strings.HasPrefix(line, "info ") {
				lastInfo = line
			}
			if strings.HasPrefix(line, "bestmove") {
				return parseResult(lastInfo, line)
			}
		case <-deadline:
			return searchResult{}, fmt.Errorf("timed out waiting for bestmove on %q", fen)
		}
	}
}

func parseResult(info, bestmove string) (searchResult, error) {
	var r searchResult

	bf := strings.Fields(bestmove)
	if len(bf) < 2 {
		return r, fmt.Errorf("unparseable bestmove line: %q", bestmove)
	}
	r.move = bf[1]

	fields := strings.Fields(info)
	for i, f := range fields {
		switch f {
		case "nodes":
			if i+1 < len(fields) {
				nodes, err := strconv.Atoi(fields[i+1])
				if err != nil {
					return r, fmt.Errorf("unparseable nodes in %q: %w", info, err)
				}
				r.nodes = nodes
			}
		case "score":
			if i+2 < len(fields) {
				r.score = fields[i+1] + " " + fields[i+2]
			}
		}
	}
	return r, nil
}

func (e *engine) close() {
	fmt.Fprintln(e.stdin, "quit")
	e.stdin.Close()
	done := make(chan struct{})
	go func() {
		e.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		e.cmd.Process.Kill()
	}
}
