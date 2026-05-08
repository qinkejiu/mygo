package main

var out_mux_in uint8

func TopModule(c bool, d bool) {
	var cbit uint8
	var dbit uint8
	if c {
		cbit = 1
	}
	if d {
		dbit = 1
	}
	out_mux_in = (cbit | dbit) | (((dbit ^ 1) & 1) << 2) | ((cbit & dbit) << 3)
}

func main() {}
