package main

var prev_clk_151 bool
var state_151 uint8
var out_shift_ena bool
var out_counting bool
var out_done bool

func TopModule(clk bool, reset bool, data bool, done_counting bool, ack bool) {
	next := state_151

	switch state_151 {
	case 0:
		if data {
			next = 1
		} else {
			next = 0
		}
	case 1:
		if data {
			next = 2
		} else {
			next = 0
		}
	case 2:
		if data {
			next = 2
		} else {
			next = 3
		}
	case 3:
		if data {
			next = 4
		} else {
			next = 0
		}
	case 4:
		next = 5
	case 5:
		next = 6
	case 6:
		next = 7
	case 7:
		next = 8
	case 8:
		if done_counting {
			next = 9
		} else {
			next = 8
		}
	case 9:
		if ack {
			next = 0
		} else {
			next = 9
		}
	}

	if !prev_clk_151 && clk {
		if reset {
			state_151 = 0
		} else {
			state_151 = next
		}
	}

	prev_clk_151 = clk

	out_shift_ena = state_151 == 4 || state_151 == 5 || state_151 == 6 || state_151 == 7
	out_counting = state_151 == 8
	out_done = state_151 == 9
}

func main() {}
