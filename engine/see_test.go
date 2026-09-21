package engine

import (
	"math/rand"
	"testing"
)

// TestSEEKnownExchanges checks seeGE against hand-worked exchanges. want is the
// exact SEE value for the mover, so seeGE must hold at want and fail at want+1.
// The values are spelled in terms of pieceValues, which the tuner rewrites, so
// the exchanges stay right whatever numbers it measures.
func TestSEEKnownExchanges(t *testing.T) {
	pawn, knight, bishop, rook, queen := pieceValues[Pawn], pieceValues[Knight], pieceValues[Bishop], pieceValues[Rook], pieceValues[Queen]

	tests := []struct {
		name string
		fen  string
		move string
		want int
	}{
		{"undefended pawn", "4k3/8/8/3p4/8/8/8/3RK3 w - - 0 1", "d1d5", pawn},
		{"queen takes pawn defended by pawn", "4k3/8/2p5/3p4/8/8/8/3QK3 w - - 0 1", "d1d5", pawn - queen},
		{"rook takes pawn defended by pawn", "4k3/8/2p5/3p4/8/8/8/3RK3 w - - 0 1", "d1d5", pawn - rook},
		{"equal knight trade", "4k3/8/2p5/3n4/8/2N5/8/4K3 w - - 0 1", "c3d5", 0},
		{"bishop takes defended knight", "4k3/8/4p3/3n4/8/8/6B1/4K3 w - - 0 1", "g2d5", knight - bishop},
		{"black to move", "3qk3/8/8/8/3P4/2P5/8/4K3 b - - 0 1", "d8d4", pawn - queen},
		// The second rook only joins the exchange through the x-ray behind the
		// first: Rxd5 Rxd5 Rxd5 nets a pawn. Without x-ray handling this is a
		// rook down.
		{"rook battery x-ray", "3rk3/8/8/3p4/8/8/3R4/3RK3 w - - 0 1", "d2d5", pawn},
		{"king recaptures", "8/8/8/3p4/4k3/8/8/3RK3 w - - 0 1", "d1d5", pawn - rook},
		// The black king can't recapture: Rd1 x-rays d5 once Rd2 has moved there.
		{"king cannot recapture into x-ray", "8/8/8/3p4/4k3/8/3R4/3RK3 w - - 0 1", "d2d5", pawn},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := ParseFEN(tt.fen)
			m := ParseUCIMove(tt.move)

			if !seeGE(pos, m, tt.want) {
				t.Errorf("seeGE(%d) = false, want true", tt.want)
			}
			if seeGE(pos, m, tt.want+1) {
				t.Errorf("seeGE(%d) = true, want false", tt.want+1)
			}
		})
	}
}

// refExchange is a slow reference for the exchange on sq: the best net material
// the side to move can gain by capturing there (0 if it stops). Like SEE it
// ignores pins and checks, but a king may only capture if it isn't recaptured.
// ok is false if a promotion capture shows up, which SEE deliberately skips.
func refExchange(p Position, sq Square) (best int, ok bool) {
	var list MoveList
	GenerateMoves(p, p.SideToMove, &list)

	victim := p.PieceAt(sq)
	if victim == AllPieces || victim == King {
		return 0, true
	}

	color := p.SideToMove
	for _, m := range list.Slice() {
		if m.To() != sq {
			continue
		}
		if m.IsPromotion() {
			return 0, false
		}

		mover := p.PieceAt(m.From())
		undo := p.MakeMove(m)

		if mover != King || !p.IsAttacked(p.KingSquare(color), color^1) {
			reply, replyOK := refExchange(p, sq)
			if !replyOK {
				p.UnmakeMove(m, undo)
				return 0, false
			}
			best = max(best, pieceValues[victim]-reply)
		}

		p.UnmakeMove(m, undo)
	}

	return best, true
}

// TestSEEMatchesReference compares seeGE with refExchange for every legal
// non-promotion capture along random games, at the exact value and one above.
//
// A tiny mismatch rate is expected and allowed: SEE always recaptures with the
// cheapest attacker, which isn't optimal when that recapture uncovers an x-ray
// attacker for the opponent (e.g. a pawn leaving a diagonal that a bishop then
// uses), while the reference tries every attacker. That happens for ~0.01% of
// captures here, whereas breaking x-ray handling or the king rule produces
// 70-1400 mismatches out of the same ~107k captures, so the tolerance below
// still catches real bugs.
func TestSEEMatchesReference(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	checked, mismatches := 0, 0

	for game := 0; game < 300; game++ {
		pos := StartPos()

		for ply := 0; ply < 90; ply++ {
			var list MoveList
			GenerateLegalMoves(*pos, pos.SideToMove, &list)
			if list.count == 0 {
				break
			}

			for _, m := range list.Slice() {
				victim := pos.PieceAt(m.To())
				if victim == AllPieces || m.IsPromotion() {
					continue
				}

				p := *pos
				undo := p.MakeMove(m)
				reply, ok := refExchange(p, m.To())
				p.UnmakeMove(m, undo)
				if !ok {
					continue
				}

				want := pieceValues[victim] - reply
				checked++
				if !seeGE(pos, m, want) || seeGE(pos, m, want+1) {
					mismatches++
					if mismatches <= 5 {
						t.Logf("game %d ply %d move %s: reference SEE %d, seeGE(%d)=%v seeGE(%d)=%v",
							game, ply, m.UCI(), want, want, seeGE(pos, m, want), want+1, seeGE(pos, m, want+1))
					}
				}
			}

			pos.MakeMove(list.Slice()[rng.Intn(list.count)])
		}
	}

	t.Logf("checked %d captures, %d mismatches", checked, mismatches)
	if checked < 1000 {
		t.Fatalf("only %d captures checked, the test isn't exercising enough positions", checked)
	}
	if mismatches > checked/3000 {
		t.Fatalf("%d mismatches out of %d captures between seeGE and the reference exchange (allowed %d)",
			mismatches, checked, checked/3000)
	}
}
