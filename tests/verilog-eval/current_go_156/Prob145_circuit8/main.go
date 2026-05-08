package main

var prev_clock_145 bool
var q_145 bool
var out_p bool
var out_q bool

func TopModule(clock bool, a bool) {
	if prev_clock_145 && !clock {
		q_145 = a
	}

	out_p = (clock && a) || (!clock && q_145)
	out_q = q_145
	prev_clock_145 = clock
}

func main() {}
