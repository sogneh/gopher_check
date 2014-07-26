//-----------------------------------------------------------------------------------
// Copyright (c) 2014 Stephen J. Lovell
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
	"math/rand"
)

const (
	SLOT_COUNT   = 131072 // number of main TT slots.
	BUCKET_COUNT = 4      // number of buckets per slot

	TT_MASK = SLOT_COUNT - 1 // a set bitmask of length 17
)

var main_tt TT

// Zobrist Hashing
//
// Each possible square and piece combination is assigned a unique 64-bit integer key at startup.
// A unique hash key for a chess position can be generated by merging (via XOR) the keys for each
// piece/square combination, and merging in keys representing the side to move, castling rights,
// and any en-passant target square.

type TT [SLOT_COUNT]SLOT

type SLOT [BUCKET_COUNT]BUCKET

type BUCKET struct {
	key  uint64
	data uint64
}

// data stores the following: (54 bits total)
// depth (remaining) - 5 bits
// move - 21 bits
// bound/node type (exact, upper, lower) - 2 bits
// value - 17 bits
// search id (age of entry) - 9 bits

func (b *BUCKET) Depth() int {
	return int(b.data & uint64(31))
}

func (b *BUCKET) Move() MV {
	return MV((b.data >> 5) & uint64(2097151))
}

func (b *BUCKET) Type() int {
	return int((b.data >> 26) & uint64(3))
}

func (b *BUCKET) Value() int {
	return int((b.data >> 28) & uint64(131071))
}

func (b *BUCKET) Id() int {
	return int((b.data >> 45) & uint64(511))
}

func (tt *TT) get_slot(hash_key uint64) SLOT {
	return tt[hash_key&TT_MASK]
}

func (tt *TT) probe(brd *BRD, c, depth, alpha, beta, value int) {
	hash_key := brd.hash_key
	slot := tt.get_slot(hash_key)

	for _, bucket := range slot {
		if bucket.key^bucket.data == hash_key {
			break
		} // find an entry uncorrupted by lockless access.

	}

}

// use lockless storing to avoid concurrent write issues without incurring Lock() overhead.

func (tt *TT) store(brd *BRD, c, depth, alpha, beta, value int) {

}

var zobrist_table [13][64]uint64
var enp_table [65]uint64    // integer keys representing the en-passant target square, if any.
var side_key = random_key() // Integer key representing a change in side-to-move.

func random_key() uint64 { // create a pseudorandom 64-bit unsigned int key
	return uint64(rand.Uint32()) | (uint64(rand.Uint32()) << 32)
}

func setup_zobrist() {
	for sq := 0; sq < 64; sq++ {
		for pc := 0; pc < 12; pc++ {
			zobrist_table[pc][sq] = random_key()
		}
		zobrist_table[PC_EMPTY][sq] = 0 //
		enp_table[sq] = random_key()
	}
	enp_table[SQ_INVALID] = 0 //
}

func zobrist(pc PC, sq int) uint64 {
	return zobrist_table[pc][sq]
}
