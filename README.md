# KissengineV2

A UCI-compatible chess engine written in Go. Bitboards and magic-number move generation,
iterative-deepening alpha-beta search, tapered evaluation tuned on millions of labeled
positions with a Texel tuner. No dependencies beyond the Go standard library.

It has no GUI of its own — use it with any UCI front end (Arena, Cute Chess, En Croissant,
lichess-bot, …) or drive it by hand over stdin/stdout.

## Build and run

Requires Go (see [go.mod](go.mod)).

```sh
make build         # bin/KissengineV2-<version>
make build-windows # cross-compiled bin/KissengineV2-<version>.exe, no CGO needed
make run           # build, then start the engine on stdin/stdout
```

`make build` compiles with `GOAMD64=v3` (needs an x86-64-v3 CPU: Intel Haswell+/AMD
Excavator+) and picks up `cmd/default.pgo` for profile-guided optimization if present
(`make pgo` regenerates it); together they're worth roughly 7% over a plain `go build`.

Or without make:

```sh
go build -o KissengineV2 ./cmd
```

A quick session:

```
$ ./KissengineV2
uci
id name KissengineV2
id author Nikita Simokhin
option name Hash type spin default 256 min 1 max 4096
uciok
position startpos moves e2e4 e7e5
go depth 10
info depth 10 score cp 25 nodes ... nps ... time ... pv ...
bestmove g1f3
```

## UCI support

| Command | Notes |
|---|---|
| `uci`, `isready`, `ucinewgame`, `quit` | `ucinewgame` clears the transposition table |
| `position [startpos \| fen ...] [moves ...]` | moves are trusted and not checked for legality |
| `go` | `depth`, `movetime`, `wtime`/`btime`/`winc`/`binc`, `movestogo`, `infinite`; other parameters are ignored |
| `stop` | works while a search is running; the search runs in the background |
| `setoption name Hash value <MB>` | transposition table size, 1–4096 MB (default 256), rounded down to a power of two |

After every completed iteration the engine prints
`info depth … score cp|mate … nodes … nps … time … pv …`.

## How it plays

- **Board:** one bitboard per piece type and colour, a mailbox for O(1) piece lookup,
  incremental Zobrist hashing, make/unmake in place.
- **Move generation:** magic bitboards for sliders, precomputed knight/king/pawn attacks,
  legality checking only for the moves that can be illegal (king, pinned pieces, en passant,
  or everything when in check).
- **Search:** negamax with PVS and aspiration windows, transposition table, null-move pruning,
  reverse futility pruning, late move pruning and futility pruning near the horizon, late move
  reductions, check extensions, quiescence search with SEE pruning, killer and history
  heuristics, repetition and 50-move detection, soft/hard time management.
- **Evaluation:** material, piece-square tables, mobility, bishop pair, rooks on open files,
  passed / doubled / isolated pawns, pawn shield — all tapered middlegame/endgame weights fit
  by a Texel tuner ([tools/texel](tools/texel)) rather than hand-tuned.

The package layout is: [cmd/](cmd) is the UCI loop, [engine/](engine) is everything else.
[CLAUDE.md](CLAUDE.md) has a much more detailed tour of the internals and the reasoning
behind non-obvious decisions.

## Development

```sh
make test    # go test ./...
make vet     # go vet ./...
make fmt     # list files that gofmt would change
make clean   # rm -rf bin/
```

After touching move generation, make/unmake or hashing, the tests that matter are `TestPerft`
(reference node counts for the start position and the standard perft test positions),
`TestUnmakeMoveRoundTrip` and `TestHashMatchesFromScratch`:

```sh
go test ./engine -run 'TestPerft|TestUnmakeMoveRoundTrip|TestHashMatchesFromScratch'
```

Search speed:

```sh
go test ./engine -bench BenchmarkSearchDepth -benchtime=10x
```

Search is deterministic at a fixed depth, so node count and score at `go depth N` are a cheap
check that a refactor did not change behaviour.

### Strength testing

[tools/match.sh](tools/match.sh) plays the working tree against a git ref with
[cutechess-cli](https://github.com/cutechess/cutechess):

```sh
tools/match.sh [BASE_REF]               # default: HEAD
GAMES=400 CONC=4 TC=10+0.1 HASH=64 tools/match.sh main~3
```

Openings come from [testdata/openings-8ply.epd](testdata/openings-8ply.epd) (400 seeded random
positions; regenerate with `go run ./tools/genopenings testdata/openings-8ply.epd`). Results
land in `bin/match-<timestamp>/match.pgn`. Mind memory: each engine process needs roughly
`HASH + 50` MB, and the script refuses to start if that times the concurrency does not fit.

### Evaluation tuning

[tools/texel](tools/texel) fits the evaluation weights with Texel's tuning method on
labeled quiet positions (gitignored, not included in the repo):

```sh
go run ./tools/texel -data data/quiet-labeled.epd,data/lichess-big3-resolved.book
```

It writes [engine/eval_params.go](engine/eval_params.go), a generated weight table the
engine loads at startup; it isn't run as part of a normal build.

## Status

A hobby engine that is still being tuned; there is no published Elo rating.
