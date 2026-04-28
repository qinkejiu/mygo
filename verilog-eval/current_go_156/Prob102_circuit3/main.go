package main

var out_q bool

func TopModule(a bool, b bool, c bool, d bool) {
	left := false
	if a {
		left = true
	}
	if b {
		left = true
	}
	right := false
	if c {
		right = true
	}
	if d {
		right = true
	}
	out_q = false
	if left {
		if right {
			out_q = true
		}
	}
}

func main() {}
