package main

var d_last_066 uint32
var out_out uint32

func TopModule(clk bool, reset bool, in uint32) {
	if clk {
		if reset {
			out_out = 0
		} else {
			out_out |= ^in & d_last_066
		}
		d_last_066 = in
	}
}

func main() {}
