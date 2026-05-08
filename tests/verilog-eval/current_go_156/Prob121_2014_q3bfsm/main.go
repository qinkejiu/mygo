package main

var state_121 uint8
var out_z bool

func TopModule(clk bool, reset bool, x bool) {
	next := state_121

	switch state_121 {
	case 0:
		if x {
			next = 1
		}
	case 1:
		if x {
			next = 4
		}
	case 2:
		if x {
			next = 1
		}
	case 3:
		if x {
			next = 2
		} else {
			next = 1
		}
	case 4:
		if !x {
			next = 3
		}
	}

	if clk {
		if reset {
			state_121 = 0
		} else {
			state_121 = next
		}
	}

	out_z = state_121 == 3 || state_121 == 4
}

func main() {}
