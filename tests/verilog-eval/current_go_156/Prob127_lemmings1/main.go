package main

var state_127 bool
var out_walk_left bool
var out_walk_right bool

func TopModule(clk bool, areset bool, bump_left bool, bump_right bool) {
	next := state_127

	if !state_127 {
		if bump_left {
			next = true
		}
	} else if bump_right {
		next = false
	}

	if areset {
		state_127 = false
	} else if clk {
		state_127 = next
	}

	out_walk_left = !state_127
	out_walk_right = state_127
}

func main() {}
