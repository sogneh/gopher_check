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
	"sync"
)

const (
	MAX_STACK = 128
)

type SPList []SplitPoint

type SplitPoint struct {
	sync.Mutex

	parent    *SplitPoint
	master    *Worker
	brd       *Board
	stack     StackItem
	depth     int
	node_type int

	alpha int // shared
	beta  int
	best  int // shared

	slave_mask           uint32 // shared
	all_slaves_searching bool   // shared
	node_count           int    // shared

	best_move    Move // shared
	move_count   int  // shared. number of moves fully searched so far.
	cutoff_found bool // shared
}

type Stack []StackItem

type StackItem struct {
	sp    *SplitPoint
	value int
	eval  int

	pv_move      Move
	current_move Move
	first_move   Move

	killers KEntry

	hash_key uint64 // use hash key to search for repetitions

	ply             int
	depth           int
	extensions_left int

	skip_pruning bool
}

