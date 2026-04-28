package main

var state_129 uint8
var out_z bool

func TopModule(clk bool, aresetn bool, x bool) {
	next := state_129

	switch state_129 {
	case 0:
		if x {
			next = 1
		}
	case 1:
		if !x {
			next = 2
		}
	case 2:
		if x {
			next = 1
		} else {
			next = 0
		}
	}

	if !aresetn {
		state_129 = 0
	} else if clk {
		state_129 = next
	}

	out_z = state_129 == 2 && x
}

func main() {}
