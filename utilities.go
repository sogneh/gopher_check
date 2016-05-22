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
	"github.com/davecheney/profile"
	"math"
	"time"
)

func RunProfiledTestSuite(test_suite string, depth, timeout int) {
	defer profile.Start(profile.MemProfile).Stop()
	RunTestSuite(test_suite, depth, timeout)
}

func RunTestSuite(test_suite string, depth, timeout int) {
	print_info = false
	test := load_epd_file(test_suite)
	var move_str string
	sum, score := 0, 0
	var gt *GameTimer
	start := time.Now()
	for i, epd := range test {
		gt = NewGameTimer(0)
		gt.max_depth = depth
		gt.PerMoveStart(time.Duration(timeout) * time.Millisecond)
		move, count := Search(epd.brd, gt)
		move_str = ToSAN(epd.brd, move)
		if correct_move(epd, move_str) {
			score += 1
			fmt.Printf("-")
		} else {
			fmt.Printf("%d.", i+1)
		}
		sum += count
	}
	seconds_elapsed := time.Since(start).Seconds()
	m_nodes := float64(sum) / 1000000.0
	fmt.Printf("\n%.4fm nodes searched in %.4fs (%.4fm NPS)\n", m_nodes, seconds_elapsed, m_nodes/seconds_elapsed)

	fmt.Printf("Total score: %d/%d\n", score, len(test))
	fmt.Printf("Average Branching factor by iteration:\n")
	var branching float64
	for d := 2; d <= depth; d++ {
		branching = math.Pow(float64(nodes_per_iteration[d])/float64(nodes_per_iteration[1]), float64(1)/float64(d-1))
		fmt.Printf("%d ABF: %.4f\n", d, branching)
	}
	fmt.Printf("Overhead: %.4fm\n", float64(load_balancer.Overhead())/1000000.0)
	fmt.Printf("Timeout: %.1fs\n", float64(timeout)/1000.0)
	// fmt.Printf("PV Accuracy: %d/%d (%.2f)\n\n", pv_accuracy[1], pv_accuracy[0]+pv_accuracy[1],
	// 	float64(pv_accuracy[1])/float64(pv_accuracy[0]+pv_accuracy[1]))
}

func correct_move(epd *EPD, move_str string) bool {
	for _, a := range epd.avoid_moves {
		if move_str == a {
			return false
		}
	}
	for _, b := range epd.best_moves {
		if move_str == b {
			return true
		}
	}
	return false
}

func ResetAll() {
	main_htable.Clear()
	// reset_main_tt()
	// for _, w := range load_balancer.workers {
	// 	w.stk = NewStack()
	// }
}
