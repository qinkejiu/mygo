package main

var state uint8
var out_q uint8

func TopModule(clk bool, a bool) {
	if clk {
		if a {
			state = 4
		} else if state == 6 {
			state = 0
		} else {
			state = state + 1
		}
	}

	out_q = state & 0x7
}

func main() {}
