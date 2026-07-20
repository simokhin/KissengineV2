package main

import (
	"bufio"
	"fmt"
	"kissengine-bitboard/engine"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	pos := engine.StartPos()

	var history []uint64

	scanner := bufio.NewScanner(os.Stdin)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		switch fields[0] {
		case "uci":
			fmt.Println("id name KissengineV2")
			fmt.Println("id author Nikita Simokhin")
			fmt.Println("uciok")

		case "isready":
			fmt.Println("readyok")

		case "ucinewgame":
			pos = engine.StartPos()
			history = []uint64{pos.Hash()}

		case "position":
			pos, history = handlePosition(fields)

		case "go":
			if len(fields) > 2 && fields[1] == "depth" {
				requestedDepth, _ := strconv.Atoi(fields[2])
				move, nodes, depth, bestScore := engine.SearchDepth(*pos, requestedDepth, history)
				fmt.Printf("info depth %d nodes %d %s\n", depth, nodes, formatScore(bestScore))
				fmt.Println("bestmove", move.UCI())
			} else {
				timeLimit := computeTimeLimit(fields, pos.SideToMove)
				move, nodes, depth, bestScore := engine.SearchTimed(*pos, timeLimit, history)
				fmt.Printf("info depth %d nodes %d %s\n", depth, nodes, formatScore(bestScore))
				fmt.Println("bestmove", move.UCI())
			}
		case "quit":
			return
		}
	}
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
		matePlies := absScore - engine.MateValue
		if matePlies < 0 {
			matePlies = 0
		}
		mateMoves := matePlies/2 + 1
		if score < 0 {
			mateMoves = -mateMoves
		}
		return fmt.Sprintf("score mate %d", mateMoves)
	}

	return fmt.Sprintf("score cp %d", score)
}

// computeTimeLimit works out how long to search for from a "go" command's
// fields, handling both a fixed "movetime" and clock-based "wtime"/"btime"
// (with optional "winc"/"binc") time controls. Falls back to one second if
// neither is present.
func computeTimeLimit(fields []string, sideToMove engine.Color) time.Duration {
	var movetime, wtime, btime, winc, binc int

	for i, f := range fields {
		if i+1 >= len(fields) {
			continue
		}
		value, err := strconv.Atoi(fields[i+1])
		if err != nil {
			continue
		}

		switch f {
		case "movetime":
			movetime = value
		case "wtime":
			wtime = value
		case "btime":
			btime = value
		case "winc":
			winc = value
		case "binc":
			binc = value
		}
	}

	if movetime > 0 {
		return time.Duration(movetime) * time.Millisecond
	}

	if wtime > 0 || btime > 0 {
		myTime, myInc := wtime, winc
		if sideToMove == engine.Black {
			myTime, myInc = btime, binc
		}

		budget := myTime/30 + myInc
		if budget < 50 {
			budget = 50
		}

		return time.Duration(budget) * time.Millisecond
	}

	return time.Second
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
