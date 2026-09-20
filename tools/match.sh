#!/usr/bin/env bash
# Match the working tree ("new") against a git ref ("base") with cutechess-cli.
#
#   tools/match.sh [BASE_REF]          # default BASE_REF=HEAD
#
# Environment overrides: GAMES (200), CONC (4), TC (10+0.1), HASH (64, in MB),
# CUTECHESS (~/cutechess/build/cutechess-cli), OUT (bin/match-<timestamp>).
# Results: $OUT/match.pgn, and the final "Score of new vs base" / Elo lines on stdout.
#
# HASH is applied to every engine that advertises the UCI Hash option. A base
# ref from before that option existed keeps its fixed table (256MB packed, or
# ~640MB with the old 40-byte entries), so the two sides then get different
# table sizes; a warning says so.
set -euo pipefail
cd "$(dirname "$0")/.."

BASE=${1:-HEAD}
GAMES=${GAMES:-200}
CONC=${CONC:-4}
TC=${TC:-10+0.1}
HASH=${HASH:-64}
CUTECHESS=${CUTECHESS:-$HOME/cutechess/build/cutechess-cli}
OUT=$(realpath -m "${OUT:-bin/match-$(date +%Y%m%d%H%M%S)}")

mkdir -p "$OUT/base-src"
git archive "$BASE" | tar -x -C "$OUT/base-src"
(cd "$OUT/base-src" && go build -o ../base ./cmd)
go build -o "$OUT/new" ./cmd

has_hash_option() {
	printf 'uci\nquit\n' | "$1" | grep -q '^option name Hash '
}

# Memory an engine process needs once its table is filled: the table plus ~50MB.
# Base refs without a Hash option keep a fixed table; assume the old 640MB
# entries (the worst case) since it can't be told apart from outside.
new_args=(-engine name=new cmd="$OUT/new" proto=uci option.Hash="$HASH")
new_mb=$((HASH + 50))
if has_hash_option "$OUT/base"; then
	base_args=(-engine name=base cmd="$OUT/base" proto=uci option.Hash="$HASH")
	base_mb=$((HASH + 50))
else
	echo "warning: base $BASE has no Hash option; it keeps its fixed table while new uses ${HASH}MB" >&2
	base_args=(-engine name=base cmd="$OUT/base" proto=uci)
	base_mb=700
fi

# Every game runs one process of each engine, CONC games at a time. 12 processes
# with 640MB tables once took WSL down, so refuse to start when the machine
# can't hold them plus ~1.5GB of headroom.
need_mb=$((CONC * (new_mb + base_mb) + 1500))
avail_mb=$(awk '/MemAvailable/ {print int($2 / 1024)}' /proc/meminfo)
if [ "$avail_mb" -lt "$need_mb" ]; then
	echo "not enough memory: ${avail_mb}MB available, need ~${need_mb}MB for CONC=$CONC HASH=$HASH" >&2
	exit 1
fi

exec "$CUTECHESS" \
	"${new_args[@]}" "${base_args[@]}" \
	-each tc="$TC" \
	-openings file=testdata/openings-8ply.epd format=epd order=random \
	-repeat -games 2 -rounds $((GAMES / 2)) -concurrency "$CONC" \
	-maxmoves 200 -resign movecount=4 score=800 -draw movenumber=50 movecount=10 score=10 \
	-pgnout "$OUT/match.pgn" -recover
