package main

var prev_clk_139 bool
var state_139 uint8
var out_f bool
var out_g bool

func TopModule(clk bool, resetn bool, x bool, y bool) {
	next := state_139

	switch state_139 {
	case 0:
		next = 1
	case 1:
		next = 2
	case 2:
		if x {
			next = 3
		} else {
			next = 2
		}
	case 3:
		if x {
			next = 3
		} else {
			next = 4
		}
	case 4:
		if x {
			next = 5
		} else {
			next = 2
		}
	case 5:
		if y {
			next = 8
		} else {
			next = 6
		}
	case 6:
		if y {
			next = 8
		} else {
			next = 7
		}
	case 7:
		next = 7
	case 8:
		next = 8
	}

	if !prev_clk_139 && clk {
		if !resetn {
			state_139 = 0
		} else {
			state_139 = next
		}
	}

	prev_clk_139 = clk

	out_f = state_139 == 1
	out_g = state_139 == 5 || state_139 == 6 || state_139 == 8
}

func main() {}
