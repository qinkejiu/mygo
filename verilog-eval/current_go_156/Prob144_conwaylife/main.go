package main

var row0_144 uint16
var row1_144 uint16
var row2_144 uint16
var row3_144 uint16
var row4_144 uint16
var row5_144 uint16
var row6_144 uint16
var row7_144 uint16
var row8_144 uint16
var row9_144 uint16
var row10_144 uint16
var row11_144 uint16
var row12_144 uint16
var row13_144 uint16
var row14_144 uint16
var row15_144 uint16
var out_q [256]bool

func rotLeft16(x uint16) uint16 {
	return (x << 1) | (x >> 15)
}

func rotRight16(x uint16) uint16 {
	return (x >> 1) | (x << 15)
}

func lifeRow16(up uint16, mid uint16, down uint16) uint16 {
	a := rotRight16(up)
	b := up
	c := rotLeft16(up)
	d := rotRight16(mid)
	e := rotLeft16(mid)
	f := rotRight16(down)
	g := down
	h := rotLeft16(down)
	ones := uint16(0)
	twos := uint16(0)
	fours := uint16(0)
	eights := uint16(0)
	c1_0 := ones & a
	ones ^= a
	c2_0 := twos & c1_0
	twos ^= c1_0
	c3_0 := fours & c2_0
	fours ^= c2_0
	eights ^= c3_0
	c1_1 := ones & b
	ones ^= b
	c2_1 := twos & c1_1
	twos ^= c1_1
	c3_1 := fours & c2_1
	fours ^= c2_1
	eights ^= c3_1
	c1_2 := ones & c
	ones ^= c
	c2_2 := twos & c1_2
	twos ^= c1_2
	c3_2 := fours & c2_2
	fours ^= c2_2
	eights ^= c3_2
	c1_3 := ones & d
	ones ^= d
	c2_3 := twos & c1_3
	twos ^= c1_3
	c3_3 := fours & c2_3
	fours ^= c2_3
	eights ^= c3_3
	c1_4 := ones & e
	ones ^= e
	c2_4 := twos & c1_4
	twos ^= c1_4
	c3_4 := fours & c2_4
	fours ^= c2_4
	eights ^= c3_4
	c1_5 := ones & f
	ones ^= f
	c2_5 := twos & c1_5
	twos ^= c1_5
	c3_5 := fours & c2_5
	fours ^= c2_5
	eights ^= c3_5
	c1_6 := ones & g
	ones ^= g
	c2_6 := twos & c1_6
	twos ^= c1_6
	c3_6 := fours & c2_6
	fours ^= c2_6
	eights ^= c3_6
	c1_7 := ones & h
	ones ^= h
	c2_7 := twos & c1_7
	twos ^= c1_7
	c3_7 := fours & c2_7
	fours ^= c2_7
	eights ^= c3_7
	count2 := (^ones) & twos & (^fours) & (^eights)
	count3 := ones & twos & (^fours) & (^eights)
	return count3 | (mid & count2)
}

func TopModule(clk bool, load bool, data [16]uint16) {
	if clk {
		if load {
			row0_144 = data[0]
			row1_144 = data[1]
			row2_144 = data[2]
			row3_144 = data[3]
			row4_144 = data[4]
			row5_144 = data[5]
			row6_144 = data[6]
			row7_144 = data[7]
			row8_144 = data[8]
			row9_144 = data[9]
			row10_144 = data[10]
			row11_144 = data[11]
			row12_144 = data[12]
			row13_144 = data[13]
			row14_144 = data[14]
			row15_144 = data[15]
		} else {
			old0 := row0_144
			old1 := row1_144
			old2 := row2_144
			old3 := row3_144
			old4 := row4_144
			old5 := row5_144
			old6 := row6_144
			old7 := row7_144
			old8 := row8_144
			old9 := row9_144
			old10 := row10_144
			old11 := row11_144
			old12 := row12_144
			old13 := row13_144
			old14 := row14_144
			old15 := row15_144
			next0 := lifeRow16(old15, old0, old1)
			next1 := lifeRow16(old0, old1, old2)
			next2 := lifeRow16(old1, old2, old3)
			next3 := lifeRow16(old2, old3, old4)
			next4 := lifeRow16(old3, old4, old5)
			next5 := lifeRow16(old4, old5, old6)
			next6 := lifeRow16(old5, old6, old7)
			next7 := lifeRow16(old6, old7, old8)
			next8 := lifeRow16(old7, old8, old9)
			next9 := lifeRow16(old8, old9, old10)
			next10 := lifeRow16(old9, old10, old11)
			next11 := lifeRow16(old10, old11, old12)
			next12 := lifeRow16(old11, old12, old13)
			next13 := lifeRow16(old12, old13, old14)
			next14 := lifeRow16(old13, old14, old15)
			next15 := lifeRow16(old14, old15, old0)
			row0_144 = next0
			row1_144 = next1
			row2_144 = next2
			row3_144 = next3
			row4_144 = next4
			row5_144 = next5
			row6_144 = next6
			row7_144 = next7
			row8_144 = next8
			row9_144 = next9
			row10_144 = next10
			row11_144 = next11
			row12_144 = next12
			row13_144 = next13
			row14_144 = next14
			row15_144 = next15
		}
	}
	out_q[0] = ((row0_144 >> 0) & 1) != 0
	out_q[1] = ((row0_144 >> 1) & 1) != 0
	out_q[2] = ((row0_144 >> 2) & 1) != 0
	out_q[3] = ((row0_144 >> 3) & 1) != 0
	out_q[4] = ((row0_144 >> 4) & 1) != 0
	out_q[5] = ((row0_144 >> 5) & 1) != 0
	out_q[6] = ((row0_144 >> 6) & 1) != 0
	out_q[7] = ((row0_144 >> 7) & 1) != 0
	out_q[8] = ((row0_144 >> 8) & 1) != 0
	out_q[9] = ((row0_144 >> 9) & 1) != 0
	out_q[10] = ((row0_144 >> 10) & 1) != 0
	out_q[11] = ((row0_144 >> 11) & 1) != 0
	out_q[12] = ((row0_144 >> 12) & 1) != 0
	out_q[13] = ((row0_144 >> 13) & 1) != 0
	out_q[14] = ((row0_144 >> 14) & 1) != 0
	out_q[15] = ((row0_144 >> 15) & 1) != 0
	out_q[16] = ((row1_144 >> 0) & 1) != 0
	out_q[17] = ((row1_144 >> 1) & 1) != 0
	out_q[18] = ((row1_144 >> 2) & 1) != 0
	out_q[19] = ((row1_144 >> 3) & 1) != 0
	out_q[20] = ((row1_144 >> 4) & 1) != 0
	out_q[21] = ((row1_144 >> 5) & 1) != 0
	out_q[22] = ((row1_144 >> 6) & 1) != 0
	out_q[23] = ((row1_144 >> 7) & 1) != 0
	out_q[24] = ((row1_144 >> 8) & 1) != 0
	out_q[25] = ((row1_144 >> 9) & 1) != 0
	out_q[26] = ((row1_144 >> 10) & 1) != 0
	out_q[27] = ((row1_144 >> 11) & 1) != 0
	out_q[28] = ((row1_144 >> 12) & 1) != 0
	out_q[29] = ((row1_144 >> 13) & 1) != 0
	out_q[30] = ((row1_144 >> 14) & 1) != 0
	out_q[31] = ((row1_144 >> 15) & 1) != 0
	out_q[32] = ((row2_144 >> 0) & 1) != 0
	out_q[33] = ((row2_144 >> 1) & 1) != 0
	out_q[34] = ((row2_144 >> 2) & 1) != 0
	out_q[35] = ((row2_144 >> 3) & 1) != 0
	out_q[36] = ((row2_144 >> 4) & 1) != 0
	out_q[37] = ((row2_144 >> 5) & 1) != 0
	out_q[38] = ((row2_144 >> 6) & 1) != 0
	out_q[39] = ((row2_144 >> 7) & 1) != 0
	out_q[40] = ((row2_144 >> 8) & 1) != 0
	out_q[41] = ((row2_144 >> 9) & 1) != 0
	out_q[42] = ((row2_144 >> 10) & 1) != 0
	out_q[43] = ((row2_144 >> 11) & 1) != 0
	out_q[44] = ((row2_144 >> 12) & 1) != 0
	out_q[45] = ((row2_144 >> 13) & 1) != 0
	out_q[46] = ((row2_144 >> 14) & 1) != 0
	out_q[47] = ((row2_144 >> 15) & 1) != 0
	out_q[48] = ((row3_144 >> 0) & 1) != 0
	out_q[49] = ((row3_144 >> 1) & 1) != 0
	out_q[50] = ((row3_144 >> 2) & 1) != 0
	out_q[51] = ((row3_144 >> 3) & 1) != 0
	out_q[52] = ((row3_144 >> 4) & 1) != 0
	out_q[53] = ((row3_144 >> 5) & 1) != 0
	out_q[54] = ((row3_144 >> 6) & 1) != 0
	out_q[55] = ((row3_144 >> 7) & 1) != 0
	out_q[56] = ((row3_144 >> 8) & 1) != 0
	out_q[57] = ((row3_144 >> 9) & 1) != 0
	out_q[58] = ((row3_144 >> 10) & 1) != 0
	out_q[59] = ((row3_144 >> 11) & 1) != 0
	out_q[60] = ((row3_144 >> 12) & 1) != 0
	out_q[61] = ((row3_144 >> 13) & 1) != 0
	out_q[62] = ((row3_144 >> 14) & 1) != 0
	out_q[63] = ((row3_144 >> 15) & 1) != 0
	out_q[64] = ((row4_144 >> 0) & 1) != 0
	out_q[65] = ((row4_144 >> 1) & 1) != 0
	out_q[66] = ((row4_144 >> 2) & 1) != 0
	out_q[67] = ((row4_144 >> 3) & 1) != 0
	out_q[68] = ((row4_144 >> 4) & 1) != 0
	out_q[69] = ((row4_144 >> 5) & 1) != 0
	out_q[70] = ((row4_144 >> 6) & 1) != 0
	out_q[71] = ((row4_144 >> 7) & 1) != 0
	out_q[72] = ((row4_144 >> 8) & 1) != 0
	out_q[73] = ((row4_144 >> 9) & 1) != 0
	out_q[74] = ((row4_144 >> 10) & 1) != 0
	out_q[75] = ((row4_144 >> 11) & 1) != 0
	out_q[76] = ((row4_144 >> 12) & 1) != 0
	out_q[77] = ((row4_144 >> 13) & 1) != 0
	out_q[78] = ((row4_144 >> 14) & 1) != 0
	out_q[79] = ((row4_144 >> 15) & 1) != 0
	out_q[80] = ((row5_144 >> 0) & 1) != 0
	out_q[81] = ((row5_144 >> 1) & 1) != 0
	out_q[82] = ((row5_144 >> 2) & 1) != 0
	out_q[83] = ((row5_144 >> 3) & 1) != 0
	out_q[84] = ((row5_144 >> 4) & 1) != 0
	out_q[85] = ((row5_144 >> 5) & 1) != 0
	out_q[86] = ((row5_144 >> 6) & 1) != 0
	out_q[87] = ((row5_144 >> 7) & 1) != 0
	out_q[88] = ((row5_144 >> 8) & 1) != 0
	out_q[89] = ((row5_144 >> 9) & 1) != 0
	out_q[90] = ((row5_144 >> 10) & 1) != 0
	out_q[91] = ((row5_144 >> 11) & 1) != 0
	out_q[92] = ((row5_144 >> 12) & 1) != 0
	out_q[93] = ((row5_144 >> 13) & 1) != 0
	out_q[94] = ((row5_144 >> 14) & 1) != 0
	out_q[95] = ((row5_144 >> 15) & 1) != 0
	out_q[96] = ((row6_144 >> 0) & 1) != 0
	out_q[97] = ((row6_144 >> 1) & 1) != 0
	out_q[98] = ((row6_144 >> 2) & 1) != 0
	out_q[99] = ((row6_144 >> 3) & 1) != 0
	out_q[100] = ((row6_144 >> 4) & 1) != 0
	out_q[101] = ((row6_144 >> 5) & 1) != 0
	out_q[102] = ((row6_144 >> 6) & 1) != 0
	out_q[103] = ((row6_144 >> 7) & 1) != 0
	out_q[104] = ((row6_144 >> 8) & 1) != 0
	out_q[105] = ((row6_144 >> 9) & 1) != 0
	out_q[106] = ((row6_144 >> 10) & 1) != 0
	out_q[107] = ((row6_144 >> 11) & 1) != 0
	out_q[108] = ((row6_144 >> 12) & 1) != 0
	out_q[109] = ((row6_144 >> 13) & 1) != 0
	out_q[110] = ((row6_144 >> 14) & 1) != 0
	out_q[111] = ((row6_144 >> 15) & 1) != 0
	out_q[112] = ((row7_144 >> 0) & 1) != 0
	out_q[113] = ((row7_144 >> 1) & 1) != 0
	out_q[114] = ((row7_144 >> 2) & 1) != 0
	out_q[115] = ((row7_144 >> 3) & 1) != 0
	out_q[116] = ((row7_144 >> 4) & 1) != 0
	out_q[117] = ((row7_144 >> 5) & 1) != 0
	out_q[118] = ((row7_144 >> 6) & 1) != 0
	out_q[119] = ((row7_144 >> 7) & 1) != 0
	out_q[120] = ((row7_144 >> 8) & 1) != 0
	out_q[121] = ((row7_144 >> 9) & 1) != 0
	out_q[122] = ((row7_144 >> 10) & 1) != 0
	out_q[123] = ((row7_144 >> 11) & 1) != 0
	out_q[124] = ((row7_144 >> 12) & 1) != 0
	out_q[125] = ((row7_144 >> 13) & 1) != 0
	out_q[126] = ((row7_144 >> 14) & 1) != 0
	out_q[127] = ((row7_144 >> 15) & 1) != 0
	out_q[128] = ((row8_144 >> 0) & 1) != 0
	out_q[129] = ((row8_144 >> 1) & 1) != 0
	out_q[130] = ((row8_144 >> 2) & 1) != 0
	out_q[131] = ((row8_144 >> 3) & 1) != 0
	out_q[132] = ((row8_144 >> 4) & 1) != 0
	out_q[133] = ((row8_144 >> 5) & 1) != 0
	out_q[134] = ((row8_144 >> 6) & 1) != 0
	out_q[135] = ((row8_144 >> 7) & 1) != 0
	out_q[136] = ((row8_144 >> 8) & 1) != 0
	out_q[137] = ((row8_144 >> 9) & 1) != 0
	out_q[138] = ((row8_144 >> 10) & 1) != 0
	out_q[139] = ((row8_144 >> 11) & 1) != 0
	out_q[140] = ((row8_144 >> 12) & 1) != 0
	out_q[141] = ((row8_144 >> 13) & 1) != 0
	out_q[142] = ((row8_144 >> 14) & 1) != 0
	out_q[143] = ((row8_144 >> 15) & 1) != 0
	out_q[144] = ((row9_144 >> 0) & 1) != 0
	out_q[145] = ((row9_144 >> 1) & 1) != 0
	out_q[146] = ((row9_144 >> 2) & 1) != 0
	out_q[147] = ((row9_144 >> 3) & 1) != 0
	out_q[148] = ((row9_144 >> 4) & 1) != 0
	out_q[149] = ((row9_144 >> 5) & 1) != 0
	out_q[150] = ((row9_144 >> 6) & 1) != 0
	out_q[151] = ((row9_144 >> 7) & 1) != 0
	out_q[152] = ((row9_144 >> 8) & 1) != 0
	out_q[153] = ((row9_144 >> 9) & 1) != 0
	out_q[154] = ((row9_144 >> 10) & 1) != 0
	out_q[155] = ((row9_144 >> 11) & 1) != 0
	out_q[156] = ((row9_144 >> 12) & 1) != 0
	out_q[157] = ((row9_144 >> 13) & 1) != 0
	out_q[158] = ((row9_144 >> 14) & 1) != 0
	out_q[159] = ((row9_144 >> 15) & 1) != 0
	out_q[160] = ((row10_144 >> 0) & 1) != 0
	out_q[161] = ((row10_144 >> 1) & 1) != 0
	out_q[162] = ((row10_144 >> 2) & 1) != 0
	out_q[163] = ((row10_144 >> 3) & 1) != 0
	out_q[164] = ((row10_144 >> 4) & 1) != 0
	out_q[165] = ((row10_144 >> 5) & 1) != 0
	out_q[166] = ((row10_144 >> 6) & 1) != 0
	out_q[167] = ((row10_144 >> 7) & 1) != 0
	out_q[168] = ((row10_144 >> 8) & 1) != 0
	out_q[169] = ((row10_144 >> 9) & 1) != 0
	out_q[170] = ((row10_144 >> 10) & 1) != 0
	out_q[171] = ((row10_144 >> 11) & 1) != 0
	out_q[172] = ((row10_144 >> 12) & 1) != 0
	out_q[173] = ((row10_144 >> 13) & 1) != 0
	out_q[174] = ((row10_144 >> 14) & 1) != 0
	out_q[175] = ((row10_144 >> 15) & 1) != 0
	out_q[176] = ((row11_144 >> 0) & 1) != 0
	out_q[177] = ((row11_144 >> 1) & 1) != 0
	out_q[178] = ((row11_144 >> 2) & 1) != 0
	out_q[179] = ((row11_144 >> 3) & 1) != 0
	out_q[180] = ((row11_144 >> 4) & 1) != 0
	out_q[181] = ((row11_144 >> 5) & 1) != 0
	out_q[182] = ((row11_144 >> 6) & 1) != 0
	out_q[183] = ((row11_144 >> 7) & 1) != 0
	out_q[184] = ((row11_144 >> 8) & 1) != 0
	out_q[185] = ((row11_144 >> 9) & 1) != 0
	out_q[186] = ((row11_144 >> 10) & 1) != 0
	out_q[187] = ((row11_144 >> 11) & 1) != 0
	out_q[188] = ((row11_144 >> 12) & 1) != 0
	out_q[189] = ((row11_144 >> 13) & 1) != 0
	out_q[190] = ((row11_144 >> 14) & 1) != 0
	out_q[191] = ((row11_144 >> 15) & 1) != 0
	out_q[192] = ((row12_144 >> 0) & 1) != 0
	out_q[193] = ((row12_144 >> 1) & 1) != 0
	out_q[194] = ((row12_144 >> 2) & 1) != 0
	out_q[195] = ((row12_144 >> 3) & 1) != 0
	out_q[196] = ((row12_144 >> 4) & 1) != 0
	out_q[197] = ((row12_144 >> 5) & 1) != 0
	out_q[198] = ((row12_144 >> 6) & 1) != 0
	out_q[199] = ((row12_144 >> 7) & 1) != 0
	out_q[200] = ((row12_144 >> 8) & 1) != 0
	out_q[201] = ((row12_144 >> 9) & 1) != 0
	out_q[202] = ((row12_144 >> 10) & 1) != 0
	out_q[203] = ((row12_144 >> 11) & 1) != 0
	out_q[204] = ((row12_144 >> 12) & 1) != 0
	out_q[205] = ((row12_144 >> 13) & 1) != 0
	out_q[206] = ((row12_144 >> 14) & 1) != 0
	out_q[207] = ((row12_144 >> 15) & 1) != 0
	out_q[208] = ((row13_144 >> 0) & 1) != 0
	out_q[209] = ((row13_144 >> 1) & 1) != 0
	out_q[210] = ((row13_144 >> 2) & 1) != 0
	out_q[211] = ((row13_144 >> 3) & 1) != 0
	out_q[212] = ((row13_144 >> 4) & 1) != 0
	out_q[213] = ((row13_144 >> 5) & 1) != 0
	out_q[214] = ((row13_144 >> 6) & 1) != 0
	out_q[215] = ((row13_144 >> 7) & 1) != 0
	out_q[216] = ((row13_144 >> 8) & 1) != 0
	out_q[217] = ((row13_144 >> 9) & 1) != 0
	out_q[218] = ((row13_144 >> 10) & 1) != 0
	out_q[219] = ((row13_144 >> 11) & 1) != 0
	out_q[220] = ((row13_144 >> 12) & 1) != 0
	out_q[221] = ((row13_144 >> 13) & 1) != 0
	out_q[222] = ((row13_144 >> 14) & 1) != 0
	out_q[223] = ((row13_144 >> 15) & 1) != 0
	out_q[224] = ((row14_144 >> 0) & 1) != 0
	out_q[225] = ((row14_144 >> 1) & 1) != 0
	out_q[226] = ((row14_144 >> 2) & 1) != 0
	out_q[227] = ((row14_144 >> 3) & 1) != 0
	out_q[228] = ((row14_144 >> 4) & 1) != 0
	out_q[229] = ((row14_144 >> 5) & 1) != 0
	out_q[230] = ((row14_144 >> 6) & 1) != 0
	out_q[231] = ((row14_144 >> 7) & 1) != 0
	out_q[232] = ((row14_144 >> 8) & 1) != 0
	out_q[233] = ((row14_144 >> 9) & 1) != 0
	out_q[234] = ((row14_144 >> 10) & 1) != 0
	out_q[235] = ((row14_144 >> 11) & 1) != 0
	out_q[236] = ((row14_144 >> 12) & 1) != 0
	out_q[237] = ((row14_144 >> 13) & 1) != 0
	out_q[238] = ((row14_144 >> 14) & 1) != 0
	out_q[239] = ((row14_144 >> 15) & 1) != 0
	out_q[240] = ((row15_144 >> 0) & 1) != 0
	out_q[241] = ((row15_144 >> 1) & 1) != 0
	out_q[242] = ((row15_144 >> 2) & 1) != 0
	out_q[243] = ((row15_144 >> 3) & 1) != 0
	out_q[244] = ((row15_144 >> 4) & 1) != 0
	out_q[245] = ((row15_144 >> 5) & 1) != 0
	out_q[246] = ((row15_144 >> 6) & 1) != 0
	out_q[247] = ((row15_144 >> 7) & 1) != 0
	out_q[248] = ((row15_144 >> 8) & 1) != 0
	out_q[249] = ((row15_144 >> 9) & 1) != 0
	out_q[250] = ((row15_144 >> 10) & 1) != 0
	out_q[251] = ((row15_144 >> 11) & 1) != 0
	out_q[252] = ((row15_144 >> 12) & 1) != 0
	out_q[253] = ((row15_144 >> 13) & 1) != 0
	out_q[254] = ((row15_144 >> 14) & 1) != 0
	out_q[255] = ((row15_144 >> 15) & 1) != 0
}

func main() {}
