// Command selfplay generates NNUE training data by playing games with this
// engine's own search (classical eval — see project conversation on why:
// it's the mature, trusted evaluator right now, so it's what should label
// the first round of self-play data, not the still-unproven NNUE net).
// Positions come from the engine's own play, labeled by its own search, so
// unlike a downloaded dataset (test80-2023-11, see
// data/nnue-fit.txt etc.) they exactly match the distribution this engine
// actually reaches — at the cost of needing real compute time to generate
// enough of them.
//
// Runs single-threaded: engine.Search uses a package-level transposition
// table (engine/tt.go), so concurrent games in one process would corrupt
// each other's table. To parallelize across cores, run multiple OS
// processes instead, each with a distinct -seed and -out:
//
//	for i in $(seq 0 11); do
//	  go run ./tools/selfplay -seed $i -out data/selfplay/gen-$i.txt -games 2000 &
//	done
//	wait
//	cat data/selfplay/gen-*.txt > data/selfplay-combined.txt
//
// Output format matches the rest of the project's data pipeline exactly:
// "<FEN> | <score> | <result>" lines, both white-relative, ready for
// `bullet-utils convert --from text` (see docs/3-data.md in the bullet
// checkout and reference_bullet_trainer_setup memory).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"kissengine-bitboard/engine"
)

func main() {
	outPath := flag.String("out", "", "output file (required); appended to if it exists")
	openingsPath := flag.String("openings", "testdata/openings-8ply.epd", "starting positions, one FEN per line")
	depth := flag.Int("depth", 7, "fixed search depth for move selection and score labels")
	randomPlies := flag.Int("random-plies", 8, "random legal moves played at the start of each game (for diversity across games/processes); these plies are not recorded as training positions")
	games := flag.Int("games", 0, "number of games to play (0 = run until interrupted, e.g. Ctrl+C)")
	hashMB := flag.Int("hash", 16, "transposition table size (MB); kept small since each process needs its own")
	seed := flag.Int64("seed", time.Now().UnixNano(), "RNG seed for opening choice and the random-plies phase — vary this across parallel instances so they don't play duplicate games (the search itself is deterministic)")
	maxPlies := flag.Int("max-plies", 300, "abandon a game (no output) past this many plies without a natural result")
	resignScore := flag.Int("resign-score", 800, "resign if the score has favored one side by at least this many centipawns for -resign-plies in a row")
	resignPlies := flag.Int("resign-plies", 8, "consecutive plies at -resign-score needed to resign")
	drawMinPly := flag.Int("draw-min-ply", 80, "earliest ply the draw adjudication below can trigger")
	drawScore := flag.Int("draw-score", 10, "adjudicate a draw once the score has stayed within +/- this many centipawns for -draw-plies in a row, past -draw-min-ply")
	drawPlies := flag.Int("draw-plies", 16, "consecutive plies at -draw-score needed to adjudicate a draw")
	flag.Parse()

	if *outPath == "" {
		log.Fatal("-out is required")
	}

	openings, err := readLines(*openingsPath)
	if err != nil {
		log.Fatalf("reading -openings: %v", err)
	}
	if len(openings) == 0 {
		log.Fatalf("%s has no starting positions", *openingsPath)
	}

	out, err := os.OpenFile(*outPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("opening -out: %v", err)
	}
	defer out.Close()
	w := bufio.NewWriter(out)
	defer w.Flush()

	engine.SetHashSize(*hashMB)

	cfg := gameConfig{
		depth:       *depth,
		randomPlies: *randomPlies,
		maxPlies:    *maxPlies,
		resignScore: *resignScore,
		resignPlies: *resignPlies,
		drawMinPly:  *drawMinPly,
		drawScore:   *drawScore,
		drawPlies:   *drawPlies,
	}

	rng := rand.New(rand.NewSource(*seed))

	played, positions := 0, 0
	start := time.Now()
	for *games == 0 || played < *games {
		fen := openings[rng.Intn(len(openings))]
		recorded, result, ok := playGame(rng, fen, cfg)
		played++
		if !ok {
			continue // abandoned: too long without a result, discard
		}

		for _, p := range recorded {
			fmt.Fprintf(w, "%s | %d | %s\n", p.fen, p.score, formatResult(result))
		}
		positions += len(recorded)
		w.Flush() // one game's worth at a time, so Ctrl+C loses at most one game

		if played%100 == 0 {
			elapsed := time.Since(start)
			log.Printf("%d games, %d positions, %.1f games/sec, %.1f positions/sec",
				played, positions, float64(played)/elapsed.Seconds(), float64(positions)/elapsed.Seconds())
		}
	}

	log.Printf("done: %d games, %d positions written to %s", played, positions, *outPath)
}

type gameConfig struct {
	depth, randomPlies, maxPlies                               int
	resignScore, resignPlies, drawMinPly, drawScore, drawPlies int
}

type labeledPos struct {
	fen   string
	score int // white-relative centipawns
}

// playGame plays one game from fen and returns the quiet positions worth
// training on plus the final result (1.0/0.5/0.0, white-relative). ok is
// false if the game was abandoned (hit maxPlies without a natural result;
// discard, don't count it as a draw).
func playGame(rng *rand.Rand, fen string, cfg gameConfig) (recorded []labeledPos, result float64, ok bool) {
	engine.ClearHash()

	pos := engine.ParseFEN(fen)
	history := []uint64{pos.Hash()}

	whiteWinStreak, blackWinStreak, drawStreak := 0, 0, 0

	for ply := 0; ply < cfg.maxPlies; ply++ {
		var moveList engine.MoveList
		engine.GenerateLegalMoves(*pos, pos.SideToMove, &moveList)
		legal := moveList.Slice()

		if len(legal) == 0 {
			inCheck := pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1)
			switch {
			case !inCheck:
				return recorded, 0.5, true // stalemate
			case pos.SideToMove == engine.White:
				return recorded, 0.0, true // white is mated
			default:
				return recorded, 1.0, true // black is mated
			}
		}

		var mv engine.Move
		var whiteScore int
		random := ply < cfg.randomPlies

		if random {
			mv = legal[rng.Intn(len(legal))]
		} else {
			var score int
			mv, _, _, score = engine.SearchDepth(*pos, cfg.depth, history)
			whiteScore = score
			if pos.SideToMove == engine.Black {
				whiteScore = -whiteScore
			}

			if isQuiet(pos, mv) {
				recorded = append(recorded, labeledPos{fen: positionFEN(pos), score: whiteScore})
			}

			switch {
			case whiteScore >= cfg.resignScore:
				whiteWinStreak, blackWinStreak = whiteWinStreak+1, 0
			case whiteScore <= -cfg.resignScore:
				blackWinStreak, whiteWinStreak = blackWinStreak+1, 0
			default:
				whiteWinStreak, blackWinStreak = 0, 0
			}
			if whiteWinStreak >= cfg.resignPlies {
				return recorded, 1.0, true
			}
			if blackWinStreak >= cfg.resignPlies {
				return recorded, 0.0, true
			}

			if ply >= cfg.drawMinPly && abs(whiteScore) <= cfg.drawScore {
				drawStreak++
			} else {
				drawStreak = 0
			}
			if drawStreak >= cfg.drawPlies {
				return recorded, 0.5, true
			}
		}

		pos.MakeMove(mv)
		history = append(history, pos.Hash())

		if pos.FiftyMovesRule >= 100 {
			return recorded, 0.5, true
		}
		if countOccurrences(history, history[len(history)-1]) >= 3 {
			return recorded, 0.5, true
		}
	}

	return nil, 0, false
}

// isQuiet reports whether the position pos is worth recording as a training
// example: not in check, and the move about to be played isn't a capture
// (including en passant) — the same "quiet position" idea as
// data/quiet-labeled.epd and the filter in bullet's examples/simple.rs.
func isQuiet(pos *engine.Position, mv engine.Move) bool {
	if pos.IsAttacked(pos.KingSquare(pos.SideToMove), pos.SideToMove^1) {
		return false
	}

	movingPiece := pos.PieceAt(mv.From())
	if pos.PieceAt(mv.To()) != engine.AllPieces {
		return false // ordinary capture
	}
	if movingPiece == engine.Pawn && mv.From().File() != mv.To().File() {
		return false // en passant capture (diagonal pawn move onto an empty square)
	}
	return true
}

func countOccurrences(history []uint64, h uint64) int {
	n := 0
	for _, v := range history {
		if v == h {
			n++
		}
	}
	return n
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func formatResult(r float64) string {
	return strconv.FormatFloat(r, 'f', 1, 64)
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, sc.Err()
}

// pieceChars[color][pieceType] is the FEN letter for that piece.
var pieceChars = [2][6]string{
	engine.White: {engine.Pawn: "P", engine.Rook: "R", engine.Knight: "N", engine.Bishop: "B", engine.Queen: "Q", engine.King: "K"},
	engine.Black: {engine.Pawn: "p", engine.Rook: "r", engine.Knight: "n", engine.Bishop: "b", engine.Queen: "q", engine.King: "k"},
}

// positionFEN serializes pos to FEN. engine.Position has no exported FEN()
// method (only ParseFEN, the reverse), so this rebuilds it from PieceAt and
// the exported Colors bitboards. Fullmove number isn't tracked by Position
// at all, so it's always written as 1 — engine.ParseFEN and bullet's own
// FEN parsing both ignore it.
func positionFEN(pos *engine.Position) string {
	var sb strings.Builder

	for rank := 7; rank >= 0; rank-- {
		empty := 0
		for file := 0; file < 8; file++ {
			sq := engine.Square(rank*8 + file)
			pt := pos.PieceAt(sq)
			if pt == engine.AllPieces {
				empty++
				continue
			}
			if empty > 0 {
				sb.WriteString(strconv.Itoa(empty))
				empty = 0
			}
			color := engine.White
			if pos.Colors[engine.Black]&sq.BB() != 0 {
				color = engine.Black
			}
			sb.WriteString(pieceChars[color][pt])
		}
		if empty > 0 {
			sb.WriteString(strconv.Itoa(empty))
		}
		if rank > 0 {
			sb.WriteByte('/')
		}
	}

	if pos.SideToMove == engine.White {
		sb.WriteString(" w ")
	} else {
		sb.WriteString(" b ")
	}

	castling := ""
	if pos.Castling&engine.WhiteKingside != 0 {
		castling += "K"
	}
	if pos.Castling&engine.WhiteQueenside != 0 {
		castling += "Q"
	}
	if pos.Castling&engine.BlackKingside != 0 {
		castling += "k"
	}
	if pos.Castling&engine.BlackQueenside != 0 {
		castling += "q"
	}
	if castling == "" {
		castling = "-"
	}
	sb.WriteString(castling)

	sb.WriteString(" ")
	if pos.EnPassant == engine.NoSquare {
		sb.WriteString("-")
	} else {
		sb.WriteString(pos.EnPassant.String())
	}

	fmt.Fprintf(&sb, " %d 1", pos.FiftyMovesRule)

	return sb.String()
}
