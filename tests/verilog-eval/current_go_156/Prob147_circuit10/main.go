package main

var prev_clk bool
var state_c bool
var out_q bool
var out_state bool

func TopModule(clk bool, a bool, b bool) {
	out_state = state_c
	out_q = (a != b) != state_c

	if !prev_clk && clk {
		state_c = (a && b) || (a && state_c) || (b && state_c)
	}

	prev_clk = clk
}

func main() {}
