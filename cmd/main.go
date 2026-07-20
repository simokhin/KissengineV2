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

		case "position":
			pos = handlePosition(fields)

		case "go":
			timeLimit := computeTimeLimit(fields, pos.SideToMove)
			move, nodes, depth := engine.SearchTimed(*pos, timeLimit)
			fmt.Printf("info depth %d nodes %d\n", depth, nodes)
			fmt.Println("bestmove", move.UCI())
		case "quit":
			return
		}
	}
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
func handlePosition(fileds []string) *engine.Position {
	pos := engine.StartPos()

	movesIndex := -1
	for i, f := range fileds {
		if f == "moves" {
			movesIndex = i
			break
		}
	}

	if movesIndex != -1 {
		for _, uci := range fileds[movesIndex+1:] {
			pos.MakeMove(engine.ParseUCIMove(uci))
		}
	}

	return pos
}
