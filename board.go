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
	"fmt"
)

type Piece uint8

func (pc Piece) Value() int { return piece_values[pc] }

// When spawning new goroutines for subtree search, a deep copy of the Board struct will have to be made
// and passed to the new goroutine.  Keep this struct as small as possible.
type Board struct {
	pieces         [2][6]BB  // 768 bits
	squares        [64]Piece // 512 bits
	occupied       [2]BB     // 128 bits
	material       [2]int32  // 64  bits
	hash_key       uint64    // 64  bits
	pawn_hash_key  uint64    // 64  bits
	c              uint8     // 8   bits
	castle         uint8     // 8   bits
	enp_target     uint8     // 8 	bits
	halfmove_clock uint8     // 8 	bits
}

func EmptyBoard() *Board {
	brd := &Board{enp_target: SQ_INVALID}
	for sq := 0; sq < 64; sq++ {
		brd.squares[sq] = EMPTY
	}
	return brd
}

func (brd *Board) Evaluate() int {
	c, e := brd.c, brd.Enemy()
	return adjusted_placement(brd, c, e) - adjusted_placement(brd, e, c)
}

func (brd *Board) ValueAt(sq int) int {
	// if sq < 0 || sq > 63 {
	// 	fmt.Printf("Invalid square at: %d\n", SquareString(sq))
	// }
	return brd.squares[sq].Value()
}

func (brd *Board) TypeAt(sq int) Piece {
	return brd.squares[sq]
}

func (brd *Board) Enemy() uint8 {
	return brd.c ^ 1
}

func (brd *Board) Occupied() BB { return brd.occupied[0] | brd.occupied[1] }

func (brd *Board) Placement(c uint8) BB { return brd.occupied[c] }

func (brd *Board) Copy() *Board {
	return &Board{
		pieces:         brd.pieces,
		squares:        brd.squares,
		occupied:       brd.occupied,
		material:       brd.material,
		hash_key:       brd.hash_key,
		pawn_hash_key:  brd.pawn_hash_key,
		c:              brd.c,
		castle:         brd.castle,
		enp_target:     brd.enp_target,
		halfmove_clock: brd.halfmove_clock,
	}
}

func (brd *Board) PrintDetails() {
	fmt.Printf("hash_key: %d, pawn_hash_key: %d\n", brd.hash_key, brd.pawn_hash_key)
	fmt.Printf("castle: %d, enp_target: %d, halfmove_clock: %d\noccupied:\n", brd.castle, brd.enp_target, brd.halfmove_clock)
	for i := 0; i < 2; i++ {
		fmt.Printf("side: %d, material: %d\n", i, brd.material[i])
		brd.occupied[i].Print()
		for pc := 0; pc < 6; pc++ {
			fmt.Printf("piece: %d\n", pc)
			brd.pieces[i][pc].Print()
		}
	}
	brd.Print()
}

func (brd *Board) Print() {
	fmt.Printf("\n    A   B   C   D   E   F   G   H\n")
	fmt.Printf("  ---------------------------------\n")
	row := brd.squares[56:]
	fmt.Printf("8 ")
	brd.print_row(56, row)

	for i := 48; i >= 0; i -= 8 {
		row = brd.squares[i : i+8]
		fmt.Printf("%v ", 1+(i/8))
		brd.print_row(i, row)
	}
	fmt.Printf("    A   B   C   D   E   F   G   H\n")
}

func (brd *Board) print_row(start int, row []Piece) {
	fmt.Printf("| ")
	for i, piece := range row {
		if piece == EMPTY {
			fmt.Printf("  | ")
		} else {
			if brd.occupied[WHITE]&sq_mask_on[start+i] > 0 {
				fmt.Printf("%v | ", piece_graphics[WHITE][piece])
			} else {
				fmt.Printf("%v | ", piece_graphics[BLACK][piece])
			}
		}
	}
	fmt.Printf("\n  ---------------------------------\n")
}

var piece_graphics = [2][6]string{
	{"\u265F", "\u265E", "\u265D", "\u265C", "\u265B", "\u265A"},
	{"\u2659", "\u2658", "\u2657", "\u2656", "\u2655", "\u2654"},
}

var current_board *Board = EmptyBoard()
var piece_values = [6]int{100, 320, 333, 510, 880, 10000} // default piece values

// Parse a FEN string and return a pointer to a new Board object
// func NewBoard(fen string) *Board {

// }

// func (brd *Board) Equal(other *Board) bool {
// 	if brd.pieces == other.pieces &&
// 		 brd.squares == other.squares &&
// 		 brd.occupied == other.occupied &&
// 		 brd.material == other.material &&
// 		 brd.hash_key == other.hash_key &&
// 		 brd.pawn_hash_key == other.pawn_hash_key &&
// 		 brd.c == other.c &&
// 		 brd.castle == other.castle &&
// 		 brd.enp_target == other.enp_target &&
// 		 brd.halfmove_clock == other.halfmove_clock {
// 	 	 return true
// 	 } else {
// 	 	 return false
// 	 }
// }

const (
	A1 = iota
	B1
	C1
	D1
	E1
	F1
	G1
	H1
	A2
	B2
	C2
	D2
	E2
	F2
	G2
	H2
	A3
	B3
	C3
	D3
	E3
	F3
	G3
	H3
	A4
	B4
	C4
	D4
	E4
	F4
	G4
	H4
	A5
	B5
	C5
	D5
	E5
	F5
	G5
	H5
	A6
	B6
	C6
	D6
	E6
	F6
	G6
	H6
	A7
	B7
	C7
	D7
	E7
	F7
	G7
	H7
	A8
	B8
	C8
	D8
	E8
	F8
	G8
	H8
	SQ_INVALID
)
