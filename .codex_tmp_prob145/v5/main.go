package main

var prev_clock bool
var p_reg bool
var out_p bool
var out_q bool

func TopModule(clock bool, a bool) {
	if clock {
		p_reg = a
	}
	out_p = p_reg
	if prev_clock && !clock {
		out_q = a
	}
	prev_clock = clock
}

func main() {}
