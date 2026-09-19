package engine

// attackersTo returns every piece, of either color, that attacks sq when the
// board occupancy is occ. Sliders are looked up through occ, so a caller can
// clear squares in occ to reveal x-ray attackers standing behind them.
func attackersTo(p *Position, sq Square, occ Bitboard) Bitboard {
	return (PawnAttacks[Black][sq] & p.Pieces[Pawn] & p.Colors[White]) |
		(PawnAttacks[White][sq] & p.Pieces[Pawn] & p.Colors[Black]) |
		(KnightAttacks[sq] & p.Pieces[Knight]) |
		(KingAttacks[sq] & p.Pieces[King]) |
		(rookAttacksMagic(sq, occ) & (p.Pieces[Rook] | p.Pieces[Queen])) |
		(bishopAttacksMagic(sq, occ) & (p.Pieces[Bishop] | p.Pieces[Queen]))
}

// seeGE reports whether the static exchange evaluation of capture m is at
// least threshold: the material the mover nets if both sides keep recapturing
// on m.To() with their cheapest attacker, each stopping as soon as continuing
// would lose material. Pins and checks are ignored, as in any SEE, so the
// answer is an estimate; en passant and promotions are not evaluated and are
// treated as breaking even (a promotion has its own ordering bonus).
//
// This is the "swap" formulation used by Stockfish: instead of building the
// list of gains and folding it back, it tracks only whether the mover's result
// is still >= threshold, which allows exiting as soon as that is decided.
func seeGE(p *Position, m Move, threshold int) bool {
	from, to := m.From(), m.To()

	victim := p.PieceAt(to)
	if victim == AllPieces || m.IsPromotion() {
		return 0 >= threshold
	}

	// Even losing the capturing piece right away, would the mover still reach
	// threshold? swap is what the opponent must win back for the mover to fall short.
	swap := pieceValues[victim] - threshold
	if swap < 0 {
		return false
	}

	swap = pieceValues[p.PieceAt(from)] - swap
	if swap <= 0 {
		return true
	}

	occ := p.Pieces[AllPieces] ^ from.BB() ^ to.BB()
	stm := White
	if p.Colors[Black]&from.BB() != 0 {
		stm = Black
	}

	attackers := attackersTo(p, to, occ)
	res := 1

	for {
		stm ^= 1
		attackers &= occ

		stmAttackers := attackers & p.Colors[stm]
		if stmAttackers == 0 {
			break
		}

		res ^= 1

		if bb := stmAttackers & p.Pieces[Pawn]; bb != 0 {
			if swap = pieceValues[Pawn] - swap; swap < res {
				break
			}
			occ ^= bb.LSB().BB()
			attackers |= bishopAttacksMagic(to, occ) & (p.Pieces[Bishop] | p.Pieces[Queen])
		} else if bb := stmAttackers & p.Pieces[Knight]; bb != 0 {
			if swap = pieceValues[Knight] - swap; swap < res {
				break
			}
			occ ^= bb.LSB().BB()
		} else if bb := stmAttackers & p.Pieces[Bishop]; bb != 0 {
			if swap = pieceValues[Bishop] - swap; swap < res {
				break
			}
			occ ^= bb.LSB().BB()
			attackers |= bishopAttacksMagic(to, occ) & (p.Pieces[Bishop] | p.Pieces[Queen])
		} else if bb := stmAttackers & p.Pieces[Rook]; bb != 0 {
			if swap = pieceValues[Rook] - swap; swap < res {
				break
			}
			occ ^= bb.LSB().BB()
			attackers |= rookAttacksMagic(to, occ) & (p.Pieces[Rook] | p.Pieces[Queen])
		} else if bb := stmAttackers & p.Pieces[Queen]; bb != 0 {
			if swap = pieceValues[Queen] - swap; swap < res {
				break
			}
			occ ^= bb.LSB().BB()
			attackers |= (bishopAttacksMagic(to, occ) & (p.Pieces[Bishop] | p.Pieces[Queen])) |
				(rookAttacksMagic(to, occ) & (p.Pieces[Rook] | p.Pieces[Queen]))
		} else {
			// Only the king is left to capture. If the opponent still has an
			// attacker the capture would be illegal, so the result flips.
			if attackers&^p.Colors[stm] != 0 {
				return res^1 == 1
			}
			return res == 1
		}
	}

	return res == 1
}

// isLosingCapture reports whether capture m loses material in the exchange on
// its target square (SEE < 0). Capturing a piece worth at least as much as the
// capturer can't lose -- the opponent's best case is winning the capturer back
// and the mover can stop there -- so the exchange is only evaluated when the
// capturer is worth more than the victim. Non-captures and en passant are
// never losing.
func isLosingCapture(p *Position, m Move) bool {
	victim := p.PieceAt(m.To())
	if victim == AllPieces {
		return false
	}

	if pieceValues[victim] >= pieceValues[p.PieceAt(m.From())] {
		return false
	}

	return !seeGE(p, m, 0)
}
