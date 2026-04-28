package main

var q0_124 uint64
var q1_124 uint64
var q2_124 uint64
var q3_124 uint64
var q4_124 uint64
var q5_124 uint64
var q6_124 uint64
var q7_124 uint64
var out_q [8]uint64

func TopModule(clk bool, load bool, data [8]uint64) {
	if clk {
		if load {
			q0_124 = data[0]
			q1_124 = data[1]
			q2_124 = data[2]
			q3_124 = data[3]
			q4_124 = data[4]
			q5_124 = data[5]
			q6_124 = data[6]
			q7_124 = data[7]
		} else {
			left0 := q0_124 >> 1
			left1 := (q1_124 >> 1) | ((q0_124 & 0x1) << 63)
			left2 := (q2_124 >> 1) | ((q1_124 & 0x1) << 63)
			left3 := (q3_124 >> 1) | ((q2_124 & 0x1) << 63)
			left4 := (q4_124 >> 1) | ((q3_124 & 0x1) << 63)
			left5 := (q5_124 >> 1) | ((q4_124 & 0x1) << 63)
			left6 := (q6_124 >> 1) | ((q5_124 & 0x1) << 63)
			left7 := (q7_124 >> 1) | ((q6_124 & 0x1) << 63)

			right0 := (q0_124 << 1) | (q1_124 >> 63)
			right1 := (q1_124 << 1) | (q2_124 >> 63)
			right2 := (q2_124 << 1) | (q3_124 >> 63)
			right3 := (q3_124 << 1) | (q4_124 >> 63)
			right4 := (q4_124 << 1) | (q5_124 >> 63)
			right5 := (q5_124 << 1) | (q6_124 >> 63)
			right6 := (q6_124 << 1) | (q7_124 >> 63)
			right7 := q7_124 << 1

			q0_124 = ^((left0 & q0_124 & right0) | (^left0 & ^q0_124 & ^right0) | (left0 & ^q0_124 & ^right0))
			q1_124 = ^((left1 & q1_124 & right1) | (^left1 & ^q1_124 & ^right1) | (left1 & ^q1_124 & ^right1))
			q2_124 = ^((left2 & q2_124 & right2) | (^left2 & ^q2_124 & ^right2) | (left2 & ^q2_124 & ^right2))
			q3_124 = ^((left3 & q3_124 & right3) | (^left3 & ^q3_124 & ^right3) | (left3 & ^q3_124 & ^right3))
			q4_124 = ^((left4 & q4_124 & right4) | (^left4 & ^q4_124 & ^right4) | (left4 & ^q4_124 & ^right4))
			q5_124 = ^((left5 & q5_124 & right5) | (^left5 & ^q5_124 & ^right5) | (left5 & ^q5_124 & ^right5))
			q6_124 = ^((left6 & q6_124 & right6) | (^left6 & ^q6_124 & ^right6) | (left6 & ^q6_124 & ^right6))
			q7_124 = ^((left7 & q7_124 & right7) | (^left7 & ^q7_124 & ^right7) | (left7 & ^q7_124 & ^right7))
		}
	}

	out_q = [8]uint64{q0_124, q1_124, q2_124, q3_124, q4_124, q5_124, q6_124, q7_124}
}

func main() {}
