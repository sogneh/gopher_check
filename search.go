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

// Modified Young Brothers Wait (YBW) approach

// At each node, search the most promising (leftmost) child sequentially first,
// then send the rest of the successors to the load balancer.
// The requester then blocks until it receives a cancellation flag, a result, or an updated bound.

// Completion

// When each child completes, its result is sent back via a channel to the requester node.
// On completion, each node sends a cancellation flag indicating no more work is needed.

// Alpha Updates

// Bounds are stored locally in the call stack.
// When alpha is updated, the update is piped down to all running child requests via a channel.  For requests still
// in queue, use a closure to scope the arguments so that when a worker executes the job it does so with the latest
// bounds from the requestor.

// When an alpha update is received from the node above, use the received value to update the locally scoped alpha
// value.

// Cancellation

// Spawning behavior

// When some moves are scored far better than others, those nodes would be searched sequentially in hopes of
// achieving a cutoff without incurring communication overhead.

// Search phases

// To reduce synchronization overhead, all search below some depth threshold will be handled sequentially.

// Hash Move (Always)
// YBW/Parallel search allowed (Ply <= 5)
// IID (Depth >= 4)
// Null Move (Depth >= 3)
// Futility pruning (Depth <= 2)

import (
	"fmt"
	// "math"
	"time"
)

const (
	MAX_TIME     = 120000 // default search time limit in seconds (2m)
	MAX_DEPTH    = 12
	MAX_EXT      = 12
	SPLIT_MIN    = 4 // set > MAX_DEPTH to disable parallel search.
	F_PRUNE_MAX  = 3  // should always be less than SPLIT_MIN
	LMR_MIN      = 2
	MAX_PLY      = MAX_DEPTH + MAX_EXT
	IID_MIN      = 4
	MAX_Q_CHECKS = 2
	COMMS_MIN    = 6 // minimum depth at which to send info to GUI.
)

const (
	Y_PV = iota
	Y_CUT
	Y_ALL
)

type SearchResult struct {
	move  Move
	score int
	count int
	pv    *PV
}

type BoundUpdate struct {
	bound        int
	alpha_update bool
}

var side_to_move uint8
var search_id int
var cancel_search bool
var uci_mode bool = false
var uci_ponder bool = false
var print_info bool = true

var nodes_per_iteration [MAX_DEPTH + 1]int

func AbortSearch() {
	cancel_search = true
	if print_info {
		fmt.Println("Search aborted by GUI")
	}
}

func search_timer(timer *time.Timer) {
	select {
	case <-timer.C:
		AbortSearch()
	}
}

func Search(brd *Board, reps *RepList, depth, time_limit int) (Move, int) {
	cancel_search = false
	side_to_move = brd.c
	id_move[brd.c] = 0
	start := time.Now()
	timer := time.NewTimer(time.Duration(time_limit) * time.Millisecond)

	if search_id >= 512 { // only 9 bits are available to store the id in each TT entry.
		search_id = 1
	} else {
		search_id += 1
	}

	go search_timer(timer) // abort the current search after time_limit seconds.

	move, sum := iterative_deepening(brd, reps, depth, start)
	timer.Stop() // cancel the timer to prevent it from interfering with the next search if it's not
	// garbage collected before then.
	return move, sum
}

var id_move [2]Move
var id_score [2]int
var id_alpha, id_beta int

const (
	STEP_SIZE = 8
)

func iterative_deepening(brd *Board, reps *RepList, depth int, start time.Time) (Move, int) {
	var guess, count, sum int
	var current_pv *PV
	c := brd.c

	id_alpha, id_beta = -INF, INF // first iteration is always full-width.

	guess, count, current_pv = young_brothers_wait(brd, id_alpha, id_beta, 1, 0, MAX_EXT, true, true, Y_PV, reps)
	nodes_per_iteration[1] += count
	sum += count
	if current_pv != nil {
		id_move[c], id_score[c] = current_pv.m, guess
		current_pv.Save(brd, 1)
	}

	// id_alpha, id_beta = guess-STEP_SIZE, guess+STEP_SIZE

	var d int
	// var fail_low_count, fail_high_count int

	for d = 2; d <= depth; {

		// switch fail_low_count {
		// case 0:
		// 	id_alpha = guess - STEP_SIZE
		// case 1:
		// 	id_alpha = guess - (STEP_SIZE << 1)
		// case 2:
		// 	id_alpha = guess - (STEP_SIZE << 2)
		// case 3:
		// 	id_alpha = guess - (STEP_SIZE << 3)
		// default:
		// 	id_alpha = -INF
		// }
		// switch fail_high_count {
		// case 0:
		// 	id_beta = guess + STEP_SIZE
		// case 1:
		// 	id_beta = guess + (STEP_SIZE << 1)
		// case 2:
		// 	id_beta = guess + (STEP_SIZE << 2)
		// case 3:
		// 	id_beta = guess + (STEP_SIZE << 3)
		// default:
		// 	id_beta = INF
		// }
		// id_alpha = max(id_alpha, -INF)
		// id_beta = min(id_beta, INF)

		guess, count, current_pv = young_brothers_wait(brd, id_alpha, id_beta, d, 0, MAX_EXT, true, true, Y_PV, reps)
		sum += count

		if cancel_search {
			return id_move[c], sum
		} else if current_pv != nil {
			id_move[c], id_score[c] = current_pv.m, guess
			current_pv.Save(brd, d)
		} else {
			fmt.Printf("Nil PV returned to ID\n")
		}

		nodes_per_iteration[d] += count
		if d > COMMS_MIN && print_info && uci_mode { // don't print info for first few plys to reduce communication traffic.
			PrintInfo(guess, d, sum, time.Since(start), current_pv)
		}

		// if guess >= id_beta {
		// 	fail_high_count += 1
		// } else if guess <= id_alpha {
		// 	fail_low_count += 1
		// } else {
		d++
		// }

	}

	if print_info {
		PrintInfo(guess, depth, sum, time.Since(start), current_pv)
	}

	return id_move[c], sum
}

func young_brothers_wait(brd *Board, alpha, beta, depth, ply, extensions_left int, can_null, can_split bool, node_type int, old_reps *RepList) (int, int, *PV) {

	if depth <= 0 {
		score, sum := quiescence(brd, alpha, beta, depth, ply, MAX_Q_CHECKS, old_reps) // q-search is always sequential.
		return score, sum, nil
	}

	if cancel_search {
		return 0, 0, nil
	}

	if old_reps.Scan(brd.hash_key) {
		return 0, 1, nil
	}

	in_check := is_in_check(brd)
	if in_check && extensions_left > 0 {
		if MAX_EXT > extensions_left { // only extend after the first check.
			depth += 1
		}
		extensions_left -= 1
	}

	if brd.halfmove_clock >= 100 {
		if is_checkmate(brd, in_check) {
			return ply - MATE, 1, nil
		} else {
			return 0, 1, nil
		}
	}

	score, best := -INF, -INF
	old_alpha := alpha
	sum, count, legal_searched := 1, 0, 0
	pv := &PV{}
	reps := &RepList{uint32(brd.hash_key), old_reps}
	var next_pv *PV
	var best_move, first_move Move
	var null_depth int
	if depth > 6 {
		null_depth = depth - 3
	} else {
		null_depth = depth - 2
	}
	var hash_result int
	first_move, hash_result = main_tt.probe(brd, depth, null_depth, &alpha, &beta, &score)

	if node_type != Y_PV {
		if hash_result == CUTOFF_FOUND {
			return score, sum, nil
		} else if hash_result != AVOID_NULL {
			// Null-Move Pruning
			if !in_check && can_null && depth > 2 && in_endgame(brd) == 0 &&
				!pawns_only(brd, brd.c) && evaluate(brd, alpha, beta) >= beta {
				score, count = null_make(brd, beta, null_depth-1, ply+1, extensions_left, can_split, reps)
				sum += count
				if score >= beta {
					main_tt.store(brd, 0, depth, LOWER_BOUND, score)
					return score, sum, nil
				}
			}
		}
	}

	// No hash move available. If on the PV, use IID to get a decent first move to try.
	if hash_result == NO_MATCH && can_null && depth >= IID_MIN && node_type != Y_ALL { //&& node_type != Y_ALL {
		var local_pv *PV
		score, count, local_pv = young_brothers_wait(brd, alpha, beta, depth-2, ply, extensions_left, can_null, false, node_type, old_reps)
		sum += count
		if local_pv != nil {
			first_move = local_pv.m
		} else {
			fmt.Println("Nil PV returned in IID")
		}
	}

	var child_type int
	// If a hash move or IID move is available, try it first.
	if first_move.IsValid(brd) && avoids_check(brd, first_move, in_check) {
		switch node_type {
		case Y_PV:
			child_type = Y_PV
		case Y_CUT:
			child_type = Y_ALL
		case Y_ALL:
			child_type = Y_CUT
		}

		score, count, next_pv = ybw_make(brd, first_move, alpha, beta, depth-1, ply+1, extensions_left, can_null, can_split, child_type, reps)
		sum += count
		if score > best {
			if score > alpha {
				if score >= beta {
					store_cutoff(brd, first_move, depth, ply, count)
					main_tt.store(brd, first_move, depth, LOWER_BOUND, score)
					return score, sum, nil
				}
				alpha = score
				pv.m = first_move
				pv.next = next_pv
			}
			best_move = first_move
			best = score
		}
		legal_searched += 1
	}

	// Generate tactical (non-quiet) moves.  Good moves will be searched sequentially to establish good bounds
	// before remaining nodes are searched in parallel.
	best_moves, remaining_moves := get_best_moves(brd, in_check, &main_ktable[ply])
	var m Move
	var r_depth, r_extensions int

	for _, item := range *best_moves { // search the best moves sequentially.
		m = item.move
		if m == first_move || !avoids_check(brd, m, in_check) {
			continue
		}

		if legal_searched > 5 && node_type == Y_CUT {
			node_type = Y_ALL
		}
		child_type = determine_child_type(node_type, legal_searched)

		r_depth, r_extensions = depth, extensions_left
		if m.IsPromotion() && extensions_left > 0 {
			r_depth = depth + 1
			r_extensions = extensions_left - 1
		}

		if node_type == Y_PV && alpha > old_alpha {
			score, count, next_pv = ybw_make(brd, m, alpha, alpha+1, r_depth-1, ply+1, r_extensions, can_null, can_split, child_type, reps)
			sum += count
			if score > alpha {
				score, count, next_pv = ybw_make(brd, m, alpha, beta, r_depth-1, ply+1, r_extensions, can_null, can_split, Y_ALL, reps)
				sum += count
			}
		} else {
			score, count, next_pv = ybw_make(brd, m, alpha, beta, r_depth-1, ply+1, r_extensions, can_null, can_split, child_type, reps)
			sum += count
		}

		legal_searched += 1
		if score > best {
			if score > alpha {
				if score >= beta {
					store_cutoff(brd, m, depth, ply, count)
					main_tt.store(brd, m, depth, LOWER_BOUND, score)
					return score, sum, nil
				}
				alpha = score
				pv.m = m
				pv.next = next_pv
			}
			best_move = m
			best = score
		}
	}

	f_prune, can_reduce := false, false
	if !in_check && ply > 0 && node_type != Y_PV && alpha > 100-MATE {
		if depth <= F_PRUNE_MAX && evaluate(brd, alpha, beta)+piece_values[BISHOP] < alpha {
			f_prune = true
		}
		if depth >= LMR_MIN {
			can_reduce = true
		}
	}

	hash_key, pawn_hash_key := brd.hash_key, brd.pawn_hash_key
	castle, enp_target, halfmove_clock := brd.castle, brd.enp_target, brd.halfmove_clock

	// split_point := can_split && depth >= SPLIT_MIN
	// var child_counter int
	// result_child := make(chan SearchResult, 20)
	// var split bool

	// Delay the generation of remaining moves until all promotions, winning captures, and killer moves have been searched.
	// if a cutoff occurs, this will reduce move generation effort substantially.
	get_remaining_moves(brd, in_check, remaining_moves, &main_ktable[ply])

	for _, item := range *remaining_moves { // search remaining moves sequentially.
		m = item.move
		if m == first_move || !avoids_check(brd, m, in_check) {
			continue
		}

		make_move(brd, m)

		if f_prune && legal_searched > 0 && m.IsQuiet() && !is_passed_pawn(brd, m) && !is_in_check(brd) {
			unmake_move(brd, m, enp_target)
			brd.hash_key, brd.pawn_hash_key = hash_key, pawn_hash_key
			brd.castle, brd.enp_target, brd.halfmove_clock = castle, enp_target, halfmove_clock
			continue
		}

		if legal_searched > 5 && node_type == Y_CUT {
			node_type = Y_ALL
		}
		child_type = determine_child_type(node_type, legal_searched)

		r_depth, r_extensions = depth, extensions_left
		if m.IsPromotion() && extensions_left > 0 {
			r_depth = depth + 1
			r_extensions = extensions_left - 1
		} else if can_reduce && item.order == 0 && !is_passed_pawn(brd, m) && !is_in_check(brd) {
			r_depth = depth - 1 // Late move reductions
		}

		if node_type == Y_PV && alpha > old_alpha {
			score, count, next_pv = young_brothers_wait(brd, -alpha-1, -alpha, r_depth-1, ply+1, r_extensions, can_null, can_split, child_type, reps)
			score = -score
			sum += count
			if score > alpha {
				score, count, next_pv = young_brothers_wait(brd, -beta, -alpha, r_depth-1, ply+1, r_extensions, can_null, can_split, Y_ALL, reps)
				score = -score
				sum += count
			}
		} else {
			// // If decent bounds have been established, parallel search is possible.
			// // Make sure at least 3 nodes have been searched serially before spawning.
			// if split_point && legal_searched > 3 && node_type == Y_ALL {
			// 	child_counter++
			// 	split = true
			// 	go func(copy *Board, alpha, beta, depth, ply, extensions int, can_null bool, node_type int, reps *RepList) {
			// 		score, count, next_pv := young_brothers_wait(copy, -beta, -alpha, depth-1, ply+1, extensions, can_null, false, node_type, reps)
			// 		result_child <- SearchResult{m, -score, count, next_pv}
			// 	}(brd.Copy(), alpha, beta, r_depth, ply, r_extensions, can_null, child_type, reps)
			// } else {
				// split = false
				score, count, next_pv = young_brothers_wait(brd, -beta, -alpha, r_depth-1, ply+1, r_extensions, can_null, can_split, child_type, reps)
				sum += count
				score = -score
			// }
		}
		legal_searched += 1

		unmake_move(brd, m, enp_target) // to do: unmake move
		brd.hash_key, brd.pawn_hash_key = hash_key, pawn_hash_key
		brd.castle, brd.enp_target, brd.halfmove_clock = castle, enp_target, halfmove_clock

		// if !split {
			if score > best {
				if score > alpha {
					if score >= beta {
						store_cutoff(brd, m, depth, ply, count) // what happens on refutation of main pv?
						main_tt.store(brd, m, depth, LOWER_BOUND, score)
						return score, sum, nil
					}
					alpha = score
					pv.m = m
					pv.next = next_pv
				}
				best_move = m
				best = score
			}
		}
	// }

	// if child_counter > 0 {
	// remaining_pieces:
	// 	for {
	// 		select { // wait for a message to come in on one of the channels.
	// 		case result := <-result_child: // one of the child subtrees has been completely searched.
	// 			sum += result.count
	// 			if result.score > best {
	// 				if result.score > alpha {
	// 					if result.score >= beta {
	// 						store_cutoff(brd, result.move, depth, ply, result.count)
	// 						main_tt.store(brd, result.move, depth, LOWER_BOUND, score)
	// 						return result.score, sum, nil
	// 					}
	// 					alpha = result.score
	// 					pv.m = result.move
	// 					pv.next = result.pv
	// 				}
	// 				best_move = result.move
	// 				best = score
	// 			}
	// 			child_counter--
	// 			if child_counter == 0 {
	// 				break remaining_pieces // exit the for loop
	// 			}
	// 		}
	// 	}
	// }

	if legal_searched > 0 {
		if alpha > old_alpha {
			main_tt.store(brd, best_move, depth, EXACT, best) // local PV node found.
			pv.value = best
			return best, sum, pv
		} else {
			main_tt.store(brd, best_move, depth, UPPER_BOUND, best)
			return best, sum, nil
		}
	} else { // draw or checkmate detected.
		if in_check {
			main_tt.store(brd, 0, depth, EXACT, ply-MATE)
			return ply-MATE, sum, nil
		} else {
			main_tt.store(brd, 0, depth, EXACT, 0)
			return 0, sum, nil
		}
	}
}



// Q-Search will always be done sequentially: Q-search subtrees are taller and narrower than in the main search,
// making benefit of parallelism smaller and raising communication and synchronization overhead.
func quiescence(brd *Board, alpha, beta, depth, ply, checks_remaining int, old_reps *RepList) (int, int) {
	if cancel_search {
		return 0, 0
	}

	if old_reps.Scan(brd.hash_key) {
		return 0, 1
	}

	in_check := is_in_check(brd)
	if brd.halfmove_clock >= 100 {
		if is_checkmate(brd, in_check) {
			return ply-MATE, 1
		} else {
			return 0, 1
		}
	}

	score, best := -INF, -INF
	sum, count := 1, 0
	reps := &RepList{uint32(brd.hash_key), old_reps}
	legal_moves := false

	var m Move
	if in_check {
		checks_remaining -= 1
		best_moves, remaining_moves := &MoveList{}, &MoveList{}
		get_evasions(brd, best_moves, remaining_moves, &main_ktable[ply]) // only legal moves generated here.
		best_moves.Sort()
		for _, item := range *best_moves {
			m = item.move
			legal_moves = true
			score, count = q_make(brd, m, alpha, beta, depth-1, ply+1, checks_remaining, reps)
			sum += count
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
		remaining_moves.Sort()
		for _, item := range *remaining_moves {
			m = item.move
			legal_moves = true
			score, count = q_make(brd, m, alpha, beta, depth-1, ply+1, checks_remaining, reps)
			sum += count
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
		if !legal_moves {
			return ply-MATE, 1 // detect checkmate.
		}
	} else {

		score = evaluate(brd, alpha, beta) // stand pat

		if score > best {
			if score > alpha {
				if score >= beta {
					return score, sum
				}
				alpha = score
			}
			best = score
		}

		hash_key, pawn_hash_key := brd.hash_key, brd.pawn_hash_key
		castle, enp_target, halfmove_clock := brd.castle, brd.enp_target, brd.halfmove_clock
		best_moves := get_winning_captures(brd)
		for _, item := range *best_moves { // search the best moves sequentially.
			m = item.move
			if !avoids_check(brd, m, in_check) {
				continue // prune illegal moves
			}

			make_move(brd, m) // to do: make move
			if alpha > 100-MATE &&
				best+m.CapturedPiece().Value()+m.PromotedTo().PromoteValue()+piece_values[ROOK] < alpha &&
				!is_in_check(brd) {
				unmake_move(brd, m, enp_target) // to do: unmake move
				brd.hash_key, brd.pawn_hash_key = hash_key, pawn_hash_key
				brd.castle, brd.enp_target, brd.halfmove_clock = castle, enp_target, halfmove_clock
				continue
			}
			score, count := quiescence(brd, -beta, -alpha, depth, ply, checks_remaining, reps)
			unmake_move(brd, m, enp_target) // to do: unmake move
			brd.hash_key, brd.pawn_hash_key = hash_key, pawn_hash_key
			brd.castle, brd.enp_target, brd.halfmove_clock = castle, enp_target, halfmove_clock

			score = -score
			sum += count
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

		if checks_remaining > 0 {
			checking_moves := get_checks(brd, &main_ktable[ply])
			for _, item := range *checking_moves {
				m = item.move
				if !avoids_check(brd, m, false) {
					continue
				}
				score, count = q_make(brd, m, alpha, beta, depth-1, ply+1, checks_remaining, reps)
				sum += count
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
		}

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


func ybw_make(brd *Board, m Move, alpha, beta, depth, ply, extensions_left int, can_null, can_split bool, node_type int, reps *RepList) (int, int, *PV) {
	hash_key, pawn_hash_key := brd.hash_key, brd.pawn_hash_key
	castle, enp_target, halfmove_clock := brd.castle, brd.enp_target, brd.halfmove_clock

	make_move(brd, m) // to do: make move
	score, sum, pv := young_brothers_wait(brd, -beta, -alpha, depth, ply, extensions_left, can_null, can_split, node_type, reps)
	unmake_move(brd, m, enp_target) // to do: unmake move

	brd.hash_key, brd.pawn_hash_key = hash_key, pawn_hash_key
	brd.castle, brd.enp_target, brd.halfmove_clock = castle, enp_target, halfmove_clock
	return -score, sum, pv
}

func q_make(brd *Board, m Move, alpha, beta, depth, ply, checks_remaining int, reps *RepList) (int, int) {
	hash_key, pawn_hash_key := brd.hash_key, brd.pawn_hash_key
	castle, enp_target, halfmove_clock := brd.castle, brd.enp_target, brd.halfmove_clock

	make_move(brd, m) // to do: make move
	score, sum := quiescence(brd, -beta, -alpha, depth, ply, checks_remaining, reps)
	unmake_move(brd, m, enp_target) // to do: unmake move

	brd.hash_key, brd.pawn_hash_key = hash_key, pawn_hash_key
	brd.castle, brd.enp_target, brd.halfmove_clock = castle, enp_target, halfmove_clock
	return -score, sum
}

func null_make(brd *Board, beta, depth, ply, extensions_left int, can_split bool, reps *RepList) (int, int) {
	hash_key, enp_target := brd.hash_key, brd.enp_target
	brd.c ^= 1
	brd.hash_key ^= side_key
	brd.hash_key ^= enp_zobrist(enp_target)
	brd.enp_target = SQ_INVALID

	score, sum, _ := young_brothers_wait(brd, -beta, -beta+1, depth, ply, extensions_left, false, can_split, Y_CUT, reps)

	brd.c ^= 1
	brd.hash_key = hash_key
	brd.enp_target = enp_target
	return -score, sum
}

func store_cutoff(brd *Board, m Move, depth, ply, count int) {
	if !m.IsCapture() {
		main_htable.Store(m, brd.c, count)
		if !m.IsPromotion() { // By the time killer moves are tried, any promotions will already have been searched.
			main_ktable.Store(m, ply) // store killer move in killer list for this Goroutine.
		}
	}
}
