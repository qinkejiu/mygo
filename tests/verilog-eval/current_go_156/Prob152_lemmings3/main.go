package main

var state_152 uint8
var out_walk_left bool
var out_walk_right bool
var out_aaah bool
var out_digging bool

func TopModule(clk bool, areset bool, bump_left bool, bump_right bool, ground bool, dig bool) {
	next := state_152

	switch state_152 {
	case 0:
		if !ground {
			next = 2
		} else if dig {
			next = 4
		} else if bump_left {
			next = 1
		}
	case 1:
		if !ground {
			next = 3
		} else if dig {
			next = 5
		} else if bump_right {
			next = 0
		}
	case 2:
		if ground {
			next = 0
		}
	case 3:
		if ground {
			next = 1
		}
	case 4:
		if !ground {
			next = 2
		}
	case 5:
		if !ground {
			next = 3
		}
	}

	if areset {
		state_152 = 0
	} else if clk {
		state_152 = next
	}

	out_walk_left = state_152 == 0
	out_walk_right = state_152 == 1
	out_aaah = state_152 == 2 || state_152 == 3
	out_digging = state_152 == 4 || state_152 == 5
}

func main() {}
