package main

var q0_108 uint64
var q1_108 uint64
var q2_108 uint64
var q3_108 uint64
var q4_108 uint64
var q5_108 uint64
var q6_108 uint64
var q7_108 uint64
var out_q [8]uint64

func TopModule(clk bool, load bool, data [8]uint64) {
	if clk {
		if load {
			q0_108 = data[0]
			q1_108 = data[1]
			q2_108 = data[2]
			q3_108 = data[3]
			q4_108 = data[4]
			q5_108 = data[5]
			q6_108 = data[6]
			q7_108 = data[7]
		} else {
			left0 := q0_108 >> 1
			left1 := (q1_108 >> 1) | ((q0_108 & 0x1) << 63)
			left2 := (q2_108 >> 1) | ((q1_108 & 0x1) << 63)
			left3 := (q3_108 >> 1) | ((q2_108 & 0x1) << 63)
			left4 := (q4_108 >> 1) | ((q3_108 & 0x1) << 63)
			left5 := (q5_108 >> 1) | ((q4_108 & 0x1) << 63)
			left6 := (q6_108 >> 1) | ((q5_108 & 0x1) << 63)
			left7 := (q7_108 >> 1) | ((q6_108 & 0x1) << 63)

			right0 := (q0_108 << 1) | (q1_108 >> 63)
			right1 := (q1_108 << 1) | (q2_108 >> 63)
			right2 := (q2_108 << 1) | (q3_108 >> 63)
			right3 := (q3_108 << 1) | (q4_108 >> 63)
			right4 := (q4_108 << 1) | (q5_108 >> 63)
			right5 := (q5_108 << 1) | (q6_108 >> 63)
			right6 := (q6_108 << 1) | (q7_108 >> 63)
			right7 := q7_108 << 1

			q0_108 = left0 ^ right0
			q1_108 = left1 ^ right1
			q2_108 = left2 ^ right2
			q3_108 = left3 ^ right3
			q4_108 = left4 ^ right4
			q5_108 = left5 ^ right5
			q6_108 = left6 ^ right6
			q7_108 = left7 ^ right7
		}
	}

	out_q = [8]uint64{q0_108, q1_108, q2_108, q3_108, q4_108, q5_108, q6_108, q7_108}
}

func main() {}
