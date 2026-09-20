#!/usr/bin/env bash
# Match the working tree ("new") against a git ref ("base") with cutechess-cli.
#
#   tools/match.sh [BASE_REF]          # default BASE_REF=HEAD
#
# Environment overrides: GAMES (200), CONC (2), TC (10+0.1),
# CUTECHESS (~/cutechess/build/cutechess-cli), OUT (bin/match-<timestamp>).
# Results: $OUT/match.pgn, and the final "Score of new vs base" / Elo lines on stdout.
set -euo pipefail
cd "$(dirname "$0")/.."

BASE=${1:-HEAD}
GAMES=${GAMES:-200}
CONC=${CONC:-2}
TC=${TC:-10+0.1}
CUTECHESS=${CUTECHESS:-$HOME/cutechess/build/cutechess-cli}
OUT=${OUT:-bin/match-$(date +%Y%m%d%H%M%S)}

# Every engine process touches its whole ~650MB transposition table at startup,
# and there are 2*CONC of them. 12 processes took WSL down once, so refuse to
# start when the machine can't hold them plus ~1.5GB of headroom.
need_mb=$((CONC * 2 * 700 + 1500))
avail_mb=$(awk '/MemAvailable/ {print int($2 / 1024)}' /proc/meminfo)
if [ "$avail_mb" -lt "$need_mb" ]; then
	echo "not enough memory: ${avail_mb}MB available, need ~${need_mb}MB for CONC=$CONC" >&2
	exit 1
fi

mkdir -p "$OUT/base-src"
git archive "$BASE" | tar -x -C "$OUT/base-src"
(cd "$OUT/base-src" && go build -o ../base ./cmd)
go build -o "$OUT/new" ./cmd

exec "$CUTECHESS" \
	-engine name=new cmd="$PWD/$OUT/new" proto=uci \
	-engine name=base cmd="$PWD/$OUT/base" proto=uci \
	-each tc="$TC" \
	-openings file=testdata/openings-8ply.epd format=epd order=random \
	-repeat -games 2 -rounds $((GAMES / 2)) -concurrency "$CONC" \
	-maxmoves 200 -resign movecount=4 score=800 -draw movenumber=50 movecount=10 score=10 \
	-pgnout "$OUT/match.pgn" -recover
