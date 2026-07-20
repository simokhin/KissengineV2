package main

import (
	"bufio"
	"fmt"
	"kissengine-bitboard/engine"
	"os"
	"strings"
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
			move, nodes := engine.BestMove(*pos, 4)
			fmt.Printf("info depth 4 nodes %d\n", nodes)
			fmt.Println("bestmove", move.UCI())

		case "quit":
			return
		}
	}
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
