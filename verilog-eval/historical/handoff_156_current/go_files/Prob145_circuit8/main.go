package main

var prev_clock bool
var out_p bool
var out_q bool

func TopModule(clock bool, a bool) {
	if prev_clock && !clock {
		out_q = a
	}
	out_p = false
	if clock {
		out_p = a
	}
	prev_clock = clock
}

func main() {}
