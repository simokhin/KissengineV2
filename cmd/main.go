package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"kissengine-bitboard/engine"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	run(os.Stdin, os.Stdout)
}

// nnueWeightsPath is the fixed checkpoint the UseNNUE UCI option loads,
// relative to the engine's working directory (repo root, when run from
// tools/match.sh or cutechess-cli invoked from there). Part of the
// from-scratch NNUE experiment (see engine/nnue.go), not a real UCI
// option a released build would have.
const nnueWeightsPath = "data/nnue-v1.bin"

// uci holds the state of one UCI session.
type uci struct {
	out   io.Writer
	outMu sync.Mutex // the search goroutine and the command loop both print

	pos     *engine.Position
	history []uint64

	job *searchJob // the current or last search, nil before the first "go"
}

// searchJob is a search running in its own goroutine, so the command loop
// keeps reading stdin (stop, isready, quit) while it works.
type searchJob struct {
	cancel   context.CancelFunc
	done     chan struct{}
	infinite bool // must not finish before "stop"
}

// run serves UCI commands from in, writing replies to out, until "quit" or EOF.
func run(in io.Reader, out io.Writer) {
	u := &uci{out: out, pos: engine.StartPos()}
	u.history = []uint64{u.pos.Hash()}

	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20) // "position ... moves" lines get long

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "uci":
			u.send("id name KissengineV2")
			u.send("id author Nikita Simokhin")
			u.send("option name Hash type spin default %d min 1 max 4096", engine.DefaultHashMB)
			u.send("option name UseNNUE type check default false")
			u.send("uciok")

		case "isready":
			u.send("readyok")

		case "setoption":
			// The protocol only sends these while idle; stopping first also
			// keeps a stray one from reallocating the table under a search.
			u.stopSearch()

			// setoption name Hash value <MB>
			if len(fields) >= 5 && fields[1] == "name" && strings.EqualFold(fields[2], "Hash") && fields[3] == "value" {
				if mb, err := strconv.Atoi(fields[4]); err == nil {
					engine.SetHashSize(mb)
				}
			}

			// setoption name UseNNUE value <true/false>. Loads the fixed
			// experiment checkpoint at nnueWeightsPath on enabling; a load
			// failure leaves it off rather than searching with garbage
			// weights. This is a from-scratch experiment (see nnue.go),
			// not meant for cmd/main.go on main.
			if len(fields) >= 5 && fields[1] == "name" && strings.EqualFold(fields[2], "UseNNUE") && fields[3] == "value" {
				if strings.EqualFold(fields[4], "true") {
					if err := engine.LoadNNUEWeights(nnueWeightsPath); err == nil {
						engine.SetUseNNUE(true)
					}
				} else {
					engine.SetUseNNUE(false)
				}
			}

		case "ucinewgame":
			u.stopSearch()
			engine.ClearHash()
			u.pos = engine.StartPos()
			u.history = []uint64{u.pos.Hash()}

		case "position":
			u.stopSearch()
			u.pos, u.history = handlePosition(fields)

		case "go":
			u.stopSearch()
			u.startSearch(parseGo(fields))

		case "stop":
			u.cancelSearch()

		case "quit":
			u.stopSearch()
			return
		}
	}

	// stdin closed: a finite search still gets to finish and print its move
	// (handy when piping commands in), an infinite one can never end.
	if u.job != nil {
		if u.job.infinite {
			u.job.cancel()
		}
		<-u.job.done
	}
}

// send prints one line to the GUI.
func (u *uci) send(format string, args ...any) {
	u.outMu.Lock()
	defer u.outMu.Unlock()

	fmt.Fprintf(u.out, format+"\n", args...)
}

// cancelSearch asks the running search to finish; it then prints its bestmove itself.
func (u *uci) cancelSearch() {
	if u.job != nil {
		u.job.cancel()
	}
}

// stopSearch cancels the running search and waits until it has printed its bestmove.
func (u *uci) stopSearch() {
	if u.job != nil {
		u.job.cancel()
		<-u.job.done
		u.job = nil
	}
}

// startSearch runs a search for the current position in the background.
func (u *uci) startSearch(limits goLimits) {
	ctx, cancel := context.WithCancel(context.Background())

	// The budget is the soft limit the search aims for; the context's deadline is
	// the hard one, which a search of an unsettled position may run up to.
	var soft time.Duration
	budget, timed := limits.timeLimit(u.pos.SideToMove)
	if timed {
		var cancelTimeout context.CancelFunc
		ctx, cancelTimeout = context.WithTimeout(ctx, limits.hardLimit(u.pos.SideToMove, budget))
		parent := cancel
		cancel = func() { cancelTimeout(); parent() }

		if limits.movetime == 0 {
			soft = budget // "movetime" is exact: it is spent in full
		}
	}

	job := &searchJob{
		cancel:   cancel,
		done:     make(chan struct{}),
		infinite: limits.infinite || (!timed && limits.depth == 0),
	}
	u.job = job

	pos, history := *u.pos, u.history

	go func() {
		defer close(job.done)
		defer cancel()

		move, _ := engine.SearchSoft(ctx, pos, history, limits.depth, soft, u.printInfo)

		// UCI: an infinite search must not report a move until it is stopped,
		// even if it ran out of depth first.
		if job.infinite {
			<-ctx.Done()
		}

		u.send("bestmove %s", moveString(move))
	}()
}

// printInfo reports one completed iteration.
func (u *uci) printInfo(info engine.Info) {
	var nps uint64
	if ms := uint64(info.Time.Milliseconds()); ms > 0 {
		nps = info.Nodes * 1000 / ms
	}

	pv := make([]string, len(info.PV))
	for i, m := range info.PV {
		pv[i] = m.UCI()
	}

	u.send("info depth %d %s nodes %d nps %d time %d pv %s",
		info.Depth, formatScore(info.Score), info.Nodes, nps, info.Time.Milliseconds(), strings.Join(pv, " "))
}

// moveString is m in UCI notation, or "0000" when there is no move (the
// position is mate or stalemate).
func moveString(m engine.Move) string {
	if m == 0 {
		return "0000"
	}
	return m.UCI()
}

// formatScore formats a search score in UCI notation: "score mate N" if the
// score represents a forced mate (N full moves away, negative if we're the
// one getting mated), otherwise "score cp X" (centipawns).
func formatScore(score int) string {
	absScore := score
	if absScore < 0 {
		absScore = -absScore
	}

	if absScore >= engine.MateValue-1000 {
		// A mate score is MateValue - ply, where ply is the distance to the
		// mated position: 1 or 2 plies is mate in 1, 3 or 4 is mate in 2, ...
		matePlies := engine.MateValue - absScore
		mateMoves := (matePlies + 1) / 2
		if score < 0 {
			mateMoves = -mateMoves
		}
		return fmt.Sprintf("score mate %d", mateMoves)
	}

	return fmt.Sprintf("score cp %d", score)
}

// goLimits are the parameters of a "go" command that the engine understands.
type goLimits struct {
	depth     int
	movetime  int // ms
	wtime     int // ms
	btime     int
	winc      int
	binc      int
	movestogo int
	hasClock  bool // wtime or btime was given (even if 0)
	infinite  bool
}

// parseGo reads a "go" command's parameters. Keywords it doesn't act on
// (ponder, nodes, mate, searchmoves...) are skipped.
func parseGo(fields []string) goLimits {
	var l goLimits

	for i := 1; i < len(fields); i++ {
		if fields[i] == "infinite" {
			l.infinite = true
			continue
		}

		if i+1 >= len(fields) {
			break
		}
		value, err := strconv.Atoi(fields[i+1])
		if err != nil {
			continue
		}

		switch fields[i] {
		case "depth":
			l.depth = value
		case "movetime":
			l.movetime = value
		case "wtime":
			l.wtime, l.hasClock = value, true
		case "btime":
			l.btime, l.hasClock = value, true
		case "winc":
			l.winc = value
		case "binc":
			l.binc = value
		case "movestogo":
			l.movestogo = value
		default:
			continue
		}
		i++ // consumed the value
	}

	return l
}

// timeLimit works out how long to search: a fixed "movetime", or a share of
// the clock ("wtime"/"btime" with "winc"/"binc" and "movestogo"). The second
// result is false when there is no time limit (infinite, or depth-only), in
// which case the search runs until it is stopped or reaches its depth.
func (l goLimits) timeLimit(sideToMove engine.Color) (time.Duration, bool) {
	if l.infinite {
		return 0, false
	}

	if l.movetime > 0 {
		return time.Duration(l.movetime) * time.Millisecond, true
	}

	if l.hasClock {
		myTime, myInc := l.wtime, l.winc
		if sideToMove == engine.Black {
			myTime, myInc = l.btime, l.binc
		}

		// Spread the remaining time over the moves left in the control, 30
		// when unknown; never plan on fewer than 2 so one move can't take it all.
		movesLeft := 30
		if l.movestogo > 0 {
			movesLeft = min(30, max(2, l.movestogo))
		}

		budget := myTime/movesLeft + myInc
		if budget < 50 {
			budget = 50
		}

		return time.Duration(budget) * time.Millisecond, true
	}

	return 0, false
}

// hardLimit is the most a search may take when the clock decides how long:
// three times the budget, but never more than a quarter of the time left plus the
// increment (a flag loses the game), and never less than the budget. A fixed
// "movetime" is exact and has no room to grow.
func (l goLimits) hardLimit(sideToMove engine.Color, budget time.Duration) time.Duration {
	if l.movetime > 0 || !l.hasClock {
		return budget
	}

	myTime, myInc := l.wtime, l.winc
	if sideToMove == engine.Black {
		myTime, myInc = l.btime, l.binc
	}

	limit := min(3*budget, time.Duration(myTime/4+myInc)*time.Millisecond)

	return max(budget, limit)
}

// handlePosition parses a "position [startpos | fen <fen>]
// [moves ...]" command.
func handlePosition(fileds []string) (*engine.Position, []uint64) {
	movesIndex := -1
	for i, f := range fileds {
		if f == "moves" {
			movesIndex = i
			break
		}
	}

	end := len(fileds)
	if movesIndex != -1 {
		end = movesIndex
	}

	var pos *engine.Position
	if len(fileds) > 1 && fileds[1] == "fen" {
		pos = engine.ParseFEN(strings.Join(fileds[2:end], " "))
	} else {
		pos = engine.StartPos()
	}

	history := []uint64{pos.Hash()}

	if movesIndex != -1 {
		for _, uci := range fileds[movesIndex+1:] {
			pos.MakeMove(engine.ParseUCIMove(uci))
			history = append(history, pos.Hash())
		}
	}

	return pos, history
}
