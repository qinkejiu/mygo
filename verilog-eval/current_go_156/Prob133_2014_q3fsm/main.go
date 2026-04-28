package main

var state_133 uint8
var out_z bool

func TopModule(clk bool, reset bool, s bool, w bool) {
	next := state_133

	switch state_133 {
	case 0:
		if s {
			next = 1
		}
	case 1:
		if w {
			next = 4
		} else {
			next = 3
		}
	case 2:
		if w {
			next = 4
		} else {
			next = 3
		}
	case 3:
		if w {
			next = 6
		} else {
			next = 5
		}
	case 4:
		if w {
			next = 7
		} else {
			next = 6
		}
	case 5:
		next = 1
	case 6:
		if w {
			next = 2
		} else {
			next = 1
		}
	case 7:
		if w {
			next = 1
		} else {
			next = 2
		}
	}

	if clk {
		if reset {
			state_133 = 0
		} else {
			state_133 = next
		}
	}

	out_z = state_133 == 2
}

func main() {}
