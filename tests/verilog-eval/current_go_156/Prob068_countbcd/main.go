package main

var q_068 uint16
var out_ena uint8
var out_q uint16

func TopModule(clk bool, reset bool) {
	if clk {
		if reset {
			q_068 = 0
		} else {
			enable0 := true
			enable1 := (q_068 & 0x000f) == 0x0009
			enable2 := (q_068 & 0x00ff) == 0x0099
			enable3 := (q_068 & 0x0fff) == 0x0999

			d0 := q_068 & 0x000f
			d1 := (q_068 >> 4) & 0x000f
			d2 := (q_068 >> 8) & 0x000f
			d3 := (q_068 >> 12) & 0x000f

			if enable0 {
				if d0 == 9 {
					d0 = 0
				} else {
					d0++
				}
			}
			if enable1 {
				if d1 == 9 {
					d1 = 0
				} else {
					d1++
				}
			}
			if enable2 {
				if d2 == 9 {
					d2 = 0
				} else {
					d2++
				}
			}
			if enable3 {
				if d3 == 9 {
					d3 = 0
				} else {
					d3++
				}
			}

			q_068 = (d3 << 12) | (d2 << 8) | (d1 << 4) | d0
		}
	}

	out_q = q_068
	out_ena = 0
	if (q_068 & 0x000f) == 0x0009 {
		out_ena |= 0x1
	}
	if (q_068 & 0x00ff) == 0x0099 {
		out_ena |= 0x2
	}
	if (q_068 & 0x0fff) == 0x0999 {
		out_ena |= 0x4
	}
}

func main() {}
