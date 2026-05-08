package main

var q_state bool
var out_Q bool

func TopModule(clk bool, j bool, k bool) {
	if clk {
		q_state = (j && !q_state) || (!k && q_state)
	}

	out_Q = q_state
}

func main() {}
