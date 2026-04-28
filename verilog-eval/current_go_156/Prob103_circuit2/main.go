package main

var out_q bool

func TopModule(a bool, b bool, c bool, d bool) {
	var bit uint8
	if !a {
		bit ^= 1
	}
	if b {
		bit ^= 1
	}
	if c {
		bit ^= 1
	}
	if d {
		bit ^= 1
	}
	out_q = bit != 0
}

func main() {}
