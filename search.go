//-----------------------------------------------------------------------------------
// ♛ GopherCheck ♛
// Copyright © 2014 Stephen J. Lovell
//-----------------------------------------------------------------------------------
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
//-----------------------------------------------------------------------------------

package main

import (
	"fmt"
	"time"
)

const (
	SPLIT_MIN = 16 // set >= MAX_PLY to disable parallel search.

	MAX_TIME  = 120000 // default search time limit in milliseconds (2m)
	MAX_DEPTH = 16
	MAX_EXT   = 16
	MAX_PLY   = MAX_DEPTH + MAX_EXT

	F_PRUNE_MAX = 2 // should always be less than SPLIT_MIN
	LMR_MIN     = 2
	IID_MIN     = 4

	MIN_CHECK_DEPTH = -2
	COMMS_MIN    = 6 // minimum depth at which to send info to GUI.
)

const (
	Y_CUT = iota
	Y_ALL
	Y_PV
)

var side_to_move uint8
var search_id int

var print_info bool = true

var nodes_per_iteration [MAX_DEPTH + 1]int

var cancel chan bool

func AbortSearch() {
	if cancel != nil {
		select {
		case <-cancel: // If search was already aborted, don't try to close the closed channel.
		default:
			close(cancel)
		}
	}
}

func search_timer(timer *time.Timer) {
	select {
	case <-timer.C:
		AbortSearch()
	}
}

func Search(brd *Board, depth, time_limit int) (Move, int) {

	side_to_move = brd.c
	id_move[brd.c] = 0
	start := time.Now()
	timer := time.NewTimer(time.Duration(time_limit) * time.Millisecond)

	if search_id >= 512 { // only 9 bits are available to store the id in each TT entry.
		search_id = 1
	} else {
		search_id += 1
	}

	cancel = make(chan bool)
	go search_timer(timer) // abort the current search after time_limit seconds.

	brd.worker = load_balancer.RootWorker() // Send SPs generated by root goroutine to root worker.

	move, sum := iterative_deepening(brd, depth, start)
	timer.Stop() // cancel the timer to prevent it from interfering with the next search if it's not
	// garbage collected before then.
	return move, sum
}

var id_move [2]Move
var id_score [2]int
var id_alpha, id_beta int

func iterative_deepening(brd *Board, depth int, start time.Time) (Move, int) {
	var guess, total, sum int
	c := brd.c
	stk := brd.worker.stk
	id_alpha, id_beta = -INF, INF // first iteration is always full-width.
	in_check := brd.InCheck()

	for d := 1; d <= depth; d++ {

		stk[0].in_check = in_check
		guess, total = ybw(brd, stk, id_alpha, id_beta, d, 0, MAX_EXT, Y_PV, SP_NONE)
		sum += total

		select {
		case <-cancel:
			return id_move[c], sum
		default:
		}

		if stk[0].pv.m.IsMove() {
			id_move[c], id_score[c] = stk[0].pv.m, guess
			stk[0].pv.SavePV(brd, d, guess) // install PV to transposition table prior to next iteration.
		} else {
			fmt.Printf("Nil PV returned to ID\n")
		}

		nodes_per_iteration[d] += total
		if d > COMMS_MIN && print_info && uci_mode { // don't print info for first few plys to reduce communication traffic.
			fmt.Printf("\n")
			PrintInfo(guess, d, sum, time.Since(start), stk)
		}

	}

	if print_info {
		PrintInfo(guess, depth, sum, time.Since(start), stk)
	}

	return id_move[c], sum
}

func ybw(brd *Board, stk Stack, alpha, beta, depth, ply, extensions_left, node_type, sp_type int) (int, int) {
	
	select {
	case <-cancel:
		return NO_SCORE, 1
	default:
	}

	if depth <= 0 {
		if node_type == Y_PV {
			stk[ply].pv = nil
		}
		return quiescence(brd, stk, alpha, beta, 0, ply) // q-search is always sequential.
	}

	var this_stk *StackItem
	var in_check bool
	var sp *SplitPoint
	var pv *PV
	var selector *MoveSelector

	score, best, old_alpha := -INF, -INF, alpha
	sum := 1
	var best_move, first_move Move
	var null_depth, hash_result, eval, subtotal, total, legal_searched, child_type, r_depth, r_extensions int
	can_prune, f_prune, can_reduce := false, false, false

	// if the is_sp flag is set, a worker has just been assigned to this split point.
	// the SP master has already handled most of the pruning, so just read the latest values
	// from the SP and jump to the moves loop.
	if sp_type == SP_SERVANT {
		sp = stk[ply].sp
		sp.Lock()
		this_stk = sp.this_stk
		eval = sp.this_stk.eval
		in_check = this_stk.in_check
		selector = sp.selector
		sp.Unlock()
		goto search_moves
	}

	this_stk = &stk[ply]

	this_stk.hash_key = brd.hash_key
	if stk.IsRepetition(ply) { // check for draw by threefold repetition
		return -PAWN - ply, 1
	}

	in_check = this_stk.in_check

	if in_check && extensions_left > 0 {
		if MAX_EXT > extensions_left { // only extend after the first check.
			depth += 1
		}
		extensions_left -= 1
	}

	if brd.halfmove_clock >= 100 { // check for draw by halfmove rule
		if is_checkmate(brd, in_check) {
			return ply - MATE, 1
		} else {
			return -PAWN - ply, 1
		}
	}

	null_depth = depth - 4
	first_move, hash_result = main_tt.probe(brd, depth, null_depth, alpha, beta, &score)

	eval = evaluate(brd, alpha, beta)
	this_stk.eval = eval

	if node_type != Y_PV {
		if (hash_result & CUTOFF_FOUND) > 0 { // Hash hit valid for current bounds.
			return score, sum
		} else if !in_check && this_stk.can_null && hash_result != AVOID_NULL && depth > 2 &&
			!brd.PawnsOnly() && eval >= beta {  // Null-move pruning

			score, subtotal = null_make(brd, stk, beta, null_depth, ply, extensions_left)
			sum += subtotal
			if score >= beta && score < MIN_MATE {
				if depth >= 8 {  //  Null-move Verification search
					this_stk.can_null = false
					score, subtotal = ybw(brd, stk, beta-1, beta, null_depth-1, ply, extensions_left, node_type, SP_NONE)
					this_stk.can_null = true
					sum += subtotal
					if score >= beta && score < MIN_MATE {
						return score, sum
					}
				} else {
					return score, sum					
				}
			}
		}
	}

	// skip IID when in check?
	if !in_check && node_type == Y_PV && hash_result == NO_MATCH && depth >= IID_MIN {
		// No hash move available. Use IID to get a decent first move to try.
		score, subtotal = ybw(brd, stk, alpha, beta, depth-2, ply, extensions_left, Y_PV, SP_NONE)
		sum += subtotal
		if this_stk.pv != nil {
			first_move = this_stk.pv.m			
		}
	}

	selector = NewMoveSelector(brd, this_stk, in_check, first_move)

search_moves:

	if node_type == Y_PV { // remove any stored pv move from a previous iteration.
		pv = &PV{}
	}

	if !in_check && ply > 0 && alpha > -MIN_MATE {
		if depth <= F_PRUNE_MAX && !brd.PawnsOnly() { 
			can_prune = true
			if eval+piece_values[BISHOP] < alpha {
				f_prune = true
			}
		}
		if depth >= LMR_MIN {
			can_reduce = true
		}
	}

	memento := brd.NewMemento()

	for m, stage := selector.Next(sp_type); m != NO_MOVE; m, stage = selector.Next(sp_type) {

		may_promote := m.IsPotentialPromotion(brd)
		try_prune := can_prune && stage >= STAGE_REMAINING && legal_searched > 0 && !may_promote

		if try_prune && get_see(brd, m.From(), m.To(), EMPTY) < 0 {
			continue  // prune quiet moves that result in loss of moving piece			
		}

		make_move(brd, m)

		gives_check := brd.InCheck()

		if f_prune && try_prune && !gives_check {
			unmake_move(brd, m, memento)
			continue
		}

		child_type = determine_child_type(node_type, legal_searched)

		r_depth, r_extensions = depth, extensions_left
		if m.IsPromotion() && stage == STAGE_WINNING && extensions_left > 0 {
			r_depth = depth + 1 // extend winning promotions only
			r_extensions = extensions_left - 1
		} else if can_reduce && !may_promote && !gives_check && stage >= STAGE_REMAINING &&
			((node_type == Y_ALL && legal_searched > 2) || legal_searched > 6) {
			r_depth = depth - 1 // Late move reductions
		}

		stk[ply+1].in_check = gives_check // avoid having to recalculate in_check at beginning of search.
		total = 0
		if node_type == Y_PV && alpha > old_alpha {
			score, subtotal = ybw(brd, stk, -alpha-1, -alpha, r_depth-1, ply+1, r_extensions, child_type, SP_NONE)
			score = -score
			total += subtotal
			if score > alpha {
				score, subtotal = ybw(brd, stk, -beta, -alpha, r_depth-1, ply+1, r_extensions, Y_PV, SP_NONE)
				score = -score
				total += subtotal
			}
		} else {
			score, subtotal = ybw(brd, stk, -beta, -alpha, r_depth-1, ply+1, r_extensions, child_type, SP_NONE)
			score = -score
			total += subtotal
		}

		unmake_move(brd, m, memento)

		if sp_type != SP_NONE {
			sp.Lock()

			if brd.worker.IsCancelled() {
				if sp_type == SP_SERVANT {
					sp.Unlock()
					return NO_SCORE, 0
				}
				// if the cutoff was at this node, store the result.  If the cancellation came from a
				// parent SP, abort without completing the search.
				if sp.cancel {
					brd.worker.RemoveSP()
					best = sp.best
					best_move = sp.best_move
					sum = sp.node_count
					sp.Unlock()
					// the servant that found the cutoff has already stored the cutoff info.
					main_tt.store(brd, best_move, depth, LOWER_BOUND, best)
					return best, sum
				} else {
					brd.worker.CancelSP()
					sp.Unlock()
					return NO_SCORE, 0
				}
			}

			alpha = sp.alpha // get the latest info
			beta = sp.beta
			best = sp.best
			best_move = sp.best_move
			legal_searched = sp.legal_searched
			sum = sp.node_count
			sp.legal_searched += 1
			legal_searched += 1
			sp.node_count += total
			sum += total

			if score > best {
				best_move = m
				sp.best_move = m
				best = score
				sp.best = score

				if score > alpha {
					alpha = score
					sp.alpha = score
					if node_type == Y_PV { // will need to update this for parallel search
						pv.m, pv.value, pv.depth, pv.next = m, score, depth, stk[ply+1].pv
						this_stk.pv = pv
					}
					if score >= beta {
						if sp_type == SP_MASTER {
							brd.worker.CancelSP()
							main_tt.store(brd, m, depth, LOWER_BOUND, score)
						} else {
							sp.cancel = true
						}
						sp.Unlock()
						store_cutoff(this_stk, m, brd.c, total) // what happens on refutation of main pv?
						return score, sum
					}
				}
			}
			sp.Unlock()
		} else {
			sum += total
			if score > best {
				if score > alpha {
					if node_type == Y_PV {
						pv.m, pv.value, pv.depth, pv.next = m, score, depth, stk[ply+1].pv
						this_stk.pv = pv
					}
					if score >= beta {
						store_cutoff(this_stk, m, brd.c, total) // what happens on refutation of main pv?
						main_tt.store(brd, m, depth, LOWER_BOUND, score)
						return score, sum
					}
					alpha = score
				}
				best_move = m
				best = score
			}
			legal_searched += 1
		}
		// Determine if this would be a good location to begin searching in parallel.
		if sp_type == SP_NONE && can_split(brd, ply, depth, node_type, legal_searched, stage) {
			sp = setup_sp(brd, stk, selector, best_move, alpha, beta, best, depth, ply,
										extensions_left, legal_searched, node_type, sum)
			sp_type = SP_MASTER
		}
	} // end of moves loop

	switch sp_type {
	case SP_MASTER:
		brd.worker.RemoveSP() // prevent more workers from being assigned here.

		// Helpful Master Concept:
		// All moves at this SP may have been consumed, but servant workers may still be busy evaluating
		// subtrees rooted at this SP.  If that's the case, offer to help only those workers assigned to this
		// split point.
		if !sp.cancel {
			brd.worker.HelpServants(sp)
		}

		sp.Lock()
		best = sp.best
		best_move = sp.best_move
		alpha = sp.alpha
		sum = sp.node_count
		sp.Unlock()

	case SP_SERVANT:
		return NO_SCORE, 0
	default:
	}

	if legal_searched > 0 {
		if node_type == Y_PV {
			pv.m, pv.value, pv.depth, pv.next = best_move, best, depth, stk[ply+1].pv
			this_stk.pv = pv
		}
		if alpha > old_alpha {
			main_tt.store(brd, best_move, depth, EXACT, best)
			return best, sum
		} else {
			main_tt.store(brd, best_move, depth, UPPER_BOUND, best)
			return best, sum
		}
	} else {
		if in_check { // Checkmate.
			main_tt.store(brd, NO_MOVE, depth, EXACT, ply-MATE)
			return ply - MATE, sum
		} else { // Draw.
			main_tt.store(brd, NO_MOVE, depth, EXACT, 0)
			return 0, sum
		}
	}
}


// Q-Search will always be done sequentially: Q-search subtrees are taller and narrower than in the main search,
// making benefit of parallelism smaller and raising communication and synchronization overhead.
func quiescence(brd *Board, stk Stack, alpha, beta, depth, ply int) (int, int) {

	this_stk := &stk[ply]
	this_stk.hash_key = brd.hash_key
	if stk.IsRepetition(ply) {
		return -PAWN - ply, 1
	}

	in_check := this_stk.in_check
	if brd.halfmove_clock >= 100 {
		if is_checkmate(brd, in_check) {
			return ply - MATE, 1
		} else {
			return -PAWN - ply, 1
		}
	}

	score, best, sum, total := -INF, -INF, 1, 0

	if !in_check {
		score = evaluate(brd, alpha, beta) // stand pat
		this_stk.eval = score
		if score > best {
			if score > alpha {
				if score >= beta {
					return score, sum
				}
				alpha = score
			}
			best = score
		}
	}

	legal_moves := false
	memento := brd.NewMemento()
	selector := NewQMoveSelector(brd, this_stk, in_check, depth >= MIN_CHECK_DEPTH)

	var may_promote, gives_check bool
	for m := selector.Next(false); m != NO_MOVE; m = selector.Next(false) {

		may_promote = m.IsPotentialPromotion(brd)

		make_move(brd, m)

		gives_check = brd.InCheck()

		if !in_check && !gives_check && !may_promote && alpha > -MIN_MATE && 
			best+m.CapturedPiece().Value()+piece_values[ROOK] < alpha {
			unmake_move(brd, m, memento)
			continue
		}

		stk[ply+1].in_check = gives_check // avoid having to recalculate in_check at beginning of search.

		score, total = quiescence(brd, stk, -beta, -alpha, depth-1, ply+1)
		score = -score
		sum += total
		unmake_move(brd, m, memento)

		if score > best {
			if score > alpha {
				if score >= beta {
					return score, sum
				}
				alpha = score
			}
			best = score
		}
		legal_moves = true
	}

	if in_check && !legal_moves {
		return ply - MATE, 1 // detect checkmate.
	}
	return best, sum
}

func determine_child_type(node_type, legal_searched int) int {
	switch node_type {
	case Y_PV:
		if legal_searched == 0 {
			return Y_PV
		} else {
			return Y_CUT
		}
	case Y_CUT:
		if legal_searched == 0 {
			return Y_ALL
		} else {
			return Y_CUT
		}
	case Y_ALL:
		return Y_CUT
	default:
		fmt.Println("Invalid node type detected.")
		return node_type
	}
}


// Determine if the current node is a good place to start searching in parallel.
func can_split(brd *Board, ply, depth, node_type, legal_searched, stage int) bool {
	if depth >= SPLIT_MIN {
		switch node_type {
		case Y_PV:
			if ply == 0 {
				// return legal_searched > 2
				return false
			} else {
				return legal_searched > 0
			}
		case Y_CUT:
			return legal_searched > 6 && stage >= STAGE_REMAINING
		case Y_ALL:
			return legal_searched > 1
		}
	}
	return false
}

func setup_sp(brd *Board, stk Stack, ms *MoveSelector, best_move Move, alpha, beta, best, depth, ply, 
	extensions_left, legal_searched, node_type, sum int) *SplitPoint {
	master := brd.worker

	sp := &SplitPoint{
		selector: ms,
		master:   master,
		parent:   master.current_sp,

		brd:      brd.Copy(),
		this_stk: stk[ply].Copy(),

		depth:           depth,
		ply:             ply,
		extensions_left: extensions_left,

		node_type:       node_type,

		alpha:     alpha,
		beta:      beta,
		best:      best,
		best_move: best_move,

		node_count:     sum,
		legal_searched: legal_searched,
		cancel:         false,
	}

	ms.brd = sp.brd // make sure the move selector points to the static SP board.
	ms.this_stk = sp.this_stk
	stk[ply].sp = sp

	load_balancer.Lock()

	master.sp_list.Push(sp)
	master.current_sp = sp

	load_balancer.Unlock()

FlushIdle: // If there are any idle workers, assign them now.
	for {
		select {
		case w := <-load_balancer.done:
			w.assign_sp <- sp
		default:
			break FlushIdle
		}
	}
	return sp
}

func null_make(brd *Board, stk Stack, beta, null_depth, ply, extensions_left int) (int, int) {
	hash_key, enp_target := brd.hash_key, brd.enp_target
	brd.c ^= 1
	brd.hash_key ^= side_key64
	brd.hash_key ^= enp_zobrist(enp_target)
	brd.enp_target = SQ_INVALID

	// assert(brd.InCheck() == false, "Illegal position detected during null_make()")

	stk[ply+1].in_check = false // Impossible to give check from a legal position by standing pat.
	stk[ply+1].can_null = false
	score, sum := ybw(brd, stk, -beta, -beta+1, null_depth-1, ply+1, extensions_left, Y_CUT, SP_NONE)
	stk[ply+1].can_null = true

	brd.c ^= 1
	brd.hash_key = hash_key
	brd.enp_target = enp_target
	return -score, sum
}

func store_cutoff(this_stk *StackItem, m Move, c uint8, total int) {
	if m.IsQuiet() {
		main_htable.Store(m, c, total)
		this_stk.StoreKiller(m) // store killer moves in stack for this Goroutine.
	}
}
