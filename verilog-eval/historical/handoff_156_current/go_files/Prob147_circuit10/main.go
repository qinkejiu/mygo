package main

var state_c bool
var out_q bool
var out_state bool

func TopModule(clk bool, a bool, b bool) {
	if clk {
		count := 0
		if a {
			count++
		}
		if b {
			count++
		}
		if state_c {
			count++
		}
		state_c = count >= 2
	}

	out_state = state_c
	out_q = (a != b) != state_c
}

func main() {}
