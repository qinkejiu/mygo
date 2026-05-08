package main

var state_136 uint8
var out_z bool

func TopModule(clk bool, reset bool, w bool) {
	next := state_136

	switch state_136 {
	case 0:
		if !w {
			next = 1
		}
	case 1:
		if w {
			next = 3
		} else {
			next = 2
		}
	case 2:
		if w {
			next = 3
		} else {
			next = 4
		}
	case 3:
		if w {
			next = 0
		} else {
			next = 5
		}
	case 4:
		if w {
			next = 3
		}
	case 5:
		if w {
			next = 3
		} else {
			next = 2
		}
	}

	if clk {
		if reset {
			state_136 = 0
		} else {
			state_136 = next
		}
	}

	out_z = state_136 == 4 || state_136 == 5
}

func main() {}
