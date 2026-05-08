package main

var state uint8
var out_g0 bool
var out_g1 bool
var out_g2 bool

func TopModule(clk bool, resetn bool, r uint8) {
	if clk {
		if !resetn {
			state = 0
			out_g0 = false
			out_g1 = false
			out_g2 = false
		} else if state == 0 {
			if (r & 0x1) != 0 {
				state = 1
				out_g0 = true
				out_g1 = false
				out_g2 = false
			} else if (r & 0x2) != 0 {
				state = 2
				out_g0 = false
				out_g1 = true
				out_g2 = false
			} else if (r & 0x4) != 0 {
				state = 3
				out_g0 = false
				out_g1 = false
				out_g2 = true
			} else {
				state = 0
				out_g0 = false
				out_g1 = false
				out_g2 = false
			}
		} else if state == 1 {
			if (r & 0x1) != 0 {
				state = 1
				out_g0 = true
				out_g1 = false
				out_g2 = false
			} else {
				state = 0
				out_g0 = false
				out_g1 = false
				out_g2 = false
			}
		} else if state == 2 {
			if (r & 0x2) != 0 {
				state = 2
				out_g0 = false
				out_g1 = true
				out_g2 = false
			} else {
				state = 0
				out_g0 = false
				out_g1 = false
				out_g2 = false
			}
		} else if state == 3 {
			if (r & 0x4) != 0 {
				state = 3
				out_g0 = false
				out_g1 = false
				out_g2 = true
			} else {
				state = 0
				out_g0 = false
				out_g1 = false
				out_g2 = false
			}
		} else {
			state = 0
			out_g0 = false
			out_g1 = false
			out_g2 = false
		}
	}
}

func main() {}
