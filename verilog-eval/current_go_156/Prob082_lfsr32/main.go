package main

var out_q uint32 = 1

func TopModule(clk bool, reset bool) {
	qNext := out_q >> 1
	if (out_q & 0x1) != 0 {
		qNext |= 0x80000000
		qNext ^= 0x00200000
		qNext ^= 0x00000002
		qNext ^= 0x00000001
	}
	if clk {
		if reset {
			out_q = 1
		} else {
			out_q = qNext
		}
	}
}

func main() {}
