package main

import (
	"fmt"
)

/* G722 C code */

/* variables for transimit quadrature mirror filter here */
var tqmf [24]int32

/* QMF filter coefficients:
scaled by a factor of 4 compared to G722 CCITT recomendation */
var h = [24]int32{
	12, -44, -44, 212, 48, -624, 128, 1448,
	-840, -3220, 3804, 15504, 15504, 3804, -3220, -840,
	1448, 128, -624, 48, 212, -44, -44, 12,
}

var xl, xh int32

/* variables for receive quadrature mirror filter here */
var accumc [11]int32
var accumd [11]int32

/* outputs of decode() */
var xout1, xout2 int32

var xs, xd int32

/* variables for encoder (hi and lo) here */

var il, szl, spl, sl, el int32

var qq4_code4_table = [16]int32{
	0, -20456, -12896, -8968, -6288, -4240, -2584, -1200,
	20456, 12896, 8968, 6288, 4240, 2584, 1200, 0,
}

var qq6_code6_table = [64]int32{
	-136, -136, -136, -136, -24808, -21904, -19008, -16704,
	-14984, -13512, -12280, -11192, -10232, -9360, -8576, -7856,
	-7192, -6576, -6000, -5456, -4944, -4464, -4008, -3576,
	-3168, -2776, -2400, -2032, -1688, -1360, -1040, -728,
	24808, 21904, 19008, 16704, 14984, 13512, 12280, 11192,
	10232, 9360, 8576, 7856, 7192, 6576, 6000, 5456,
	4944, 4464, 4008, 3576, 3168, 2776, 2400, 2032,
	1688, 1360, 1040, 728, 432, 136, -432, -136,
}

var delay_bpl [6]int32
var delay_dltx [6]int32

var wl_code_table = [16]int32{
	-60, 3042, 1198, 538, 334, 172, 58, -30,
	3042, 1198, 538, 334, 172, 58, -30, -60,
}

var ilb_table = [32]int32{
	2048, 2093, 2139, 2186, 2233, 2282, 2332, 2383,
	2435, 2489, 2543, 2599, 2656, 2714, 2774, 2834,
	2896, 2960, 3025, 3091, 3158, 3228, 3298, 3371,
	3444, 3520, 3597, 3676, 3756, 3838, 3922, 4008,
}

var nbl int32 /* delay line */
var al1, al2 int32
var plt, plt1, plt2 int32
var dlt int32
var rlt, rlt1, rlt2 int32

/* decision levels - pre-multiplied by 8, 0 to indicate end */
var decis_levl = [30]int32{
	280, 576, 880, 1200, 1520, 1864, 2208, 2584,
	2960, 3376, 3784, 4240, 4696, 5200, 5712, 6288,
	6864, 7520, 8184, 8968, 9752, 10712, 11664, 12896,
	14120, 15840, 17560, 20456, 23352, 32767,
}

var detl int32

/* quantization table 31 long to make quantl look-up easier,
last entry is for mil=30 case when wd is max */
var quant26bt_pos = [31]int32{
	61, 60, 59, 58, 57, 56, 55, 54,
	53, 52, 51, 50, 49, 48, 47, 46,
	45, 44, 43, 42, 41, 40, 39, 38,
	37, 36, 35, 34, 33, 32, 32,
}

/* quantization table 31 long to make quantl look-up easier,
last entry is for mil=30 case when wd is max */
var quant26bt_neg = [31]int32{
	63, 62, 31, 30, 29, 28, 27, 26,
	25, 24, 23, 22, 21, 20, 19, 18,
	17, 16, 15, 14, 13, 12, 11, 10,
	9, 8, 7, 6, 5, 4, 4,
}

var deth int32
var sh int32 /* this comes from adaptive predictor */
var eh int32

var qq2_code2_table = [4]int32{
	-7408, -1616, 7408, 1616,
}

var wh_code_table = [4]int32{
	798, -214, 798, -214,
}

var dh, ih int32
var nbh, szh int32
var sph, ph, yh, rh int32

var delay_dhx [6]int32
var delay_bph [6]int32

var ah1, ah2 int32
var ph1, ph2 int32
var rh1, rh2 int32

/* variables for decoder here */
var ilr, rl int32
var dec_deth, dec_detl, dec_dlt int32

var dec_del_bpl [6]int32
var dec_del_dltx [6]int32

var dec_plt, dec_plt1, dec_plt2 int32
var dec_szl, dec_spl, dec_sl int32
var dec_rlt1, dec_rlt2, dec_rlt int32
var dec_al1, dec_al2 int32
var dl int32
var dec_nbl, dec_dh, dec_nbh int32

/* variables used in filtez */
var dec_del_bph [6]int32
var dec_del_dhx [6]int32

var dec_szh int32
/* variables used in filtep */
var dec_rh1, dec_rh2 int32
var dec_ah1, dec_ah2 int32
var dec_ph, dec_sph int32

var dec_sh int32

var dec_ph1, dec_ph2 int32

func abs(n int32) int32 {
	if n >= 0 {
		return n
	}
	return -n
}

/* G722 encode function two ints in, one 8 bit output */

/* put input samples in xin1 = first value, xin2 = second value */
/* returns il and ih stored together */
func encode(xin1, xin2 int32) int32 {
	var i int
	var xa, xb int64
	var decis int32

	/* transmit quadrature mirror filters implemented here */
	h_ptr := 0
	tqmf_ptr := 0
	xa = int64(tqmf[tqmf_ptr]) * int64(h[h_ptr])
	tqmf_ptr++
	h_ptr++
	xb = int64(tqmf[tqmf_ptr]) * int64(h[h_ptr])
	tqmf_ptr++
	h_ptr++

	/* main multiply accumulate loop for samples and coefficients */
	for i = 0; i < 10; i++ {
		xa += int64(tqmf[tqmf_ptr]) * int64(h[h_ptr])
		tqmf_ptr++
		h_ptr++
		xb += int64(tqmf[tqmf_ptr]) * int64(h[h_ptr])
		tqmf_ptr++
		h_ptr++
	}

	/* final mult/accumulate */
	xa += int64(tqmf[tqmf_ptr]) * int64(h[h_ptr])
	tqmf_ptr++
	h_ptr++
	xb += int64(tqmf[tqmf_ptr]) * int64(h[h_ptr])
	h_ptr++

	/* update delay line tqmf */
	tqmf_ptr1 := tqmf_ptr - 2
	for i = 0; i < 22; i++ {
		tqmf[tqmf_ptr] = tqmf[tqmf_ptr1]
		tqmf_ptr--
		tqmf_ptr1--
	}
	tqmf[tqmf_ptr] = xin1
	tqmf_ptr--
	tqmf[tqmf_ptr] = xin2

	/* scale outputs */
	xl = int32((xa + xb) >> 15)
	xh = int32((xa - xb) >> 15)

	/* end of quadrature mirror filter code */

	/* starting with lower sub band encoder */

	/* filtez - compute predictor output section - zero section */
	szl = filtez(delay_bpl[:], delay_dltx[:])

	/* filtep - compute predictor output signal (pole section) */
	spl = filtep(rlt1, al1, rlt2, al2)

	/* compute the predictor output value in the lower sub_band encoder */
	sl = szl + spl
	el = xl - sl

	/* quantl: quantize the difference signal */
	il = quantl(el, detl)

	/* computes quantized difference signal */
	/* for invqbl, truncate by 2 lsbs, so mode = 3 */
	dlt = int32((int64(detl) * int64(qq4_code4_table[il>>2])) >> 15)

	/* logscl: updates logarithmic quant. scale factor in low sub band */
	nbl = logscl(il, nbl)

	/* scalel: compute the quantizer scale factor in the lower sub band */
	/* calling parameters nbl and 8 (constant such that scalel can be scaleh) */
	detl = scalel(nbl, 8)

	/* parrec - simple addition to compute recontructed signal for adaptive pred */
	plt = dlt + szl

	/* upzero: update zero section predictor coefficients (sixth order)*/
	/* calling parameters: dlt, dlt1, dlt2, ..., dlt6 from dlt */
	/*  bpli (linear_buffer in which all six values are delayed */
	/* return params:      updated bpli, delayed dltx */
	upzero(dlt, delay_dltx[:], delay_bpl[:])

	/* uppol2- update second predictor coefficient apl2 and delay it as al2 */
	/* calling parameters: al1, al2, plt, plt1, plt2 */
	al2 = uppol2(al1, al2, plt, plt1, plt2)

	/* uppol1 :update first predictor coefficient apl1 and delay it as al1 */
	/* calling parameters: al1, apl2, plt, plt1 */
	al1 = uppol1(al1, al2, plt, plt1)

	/* recons : compute recontructed signal for adaptive predictor */
	rlt = sl + dlt

	/* done with lower sub_band encoder; now implement delays for next time*/
	rlt2 = rlt1
	rlt1 = rlt
	plt2 = plt1
	plt1 = plt

	/* high band encode */

	szh = filtez(delay_bph[:], delay_dhx[:])

	sph = filtep(rh1, ah1, rh2, ah2)

	/* predic: sh = sph + szh */
	sh = sph + szh
	/* subtra: eh = xh - sh */
	eh = xh - sh

	/* quanth - quantization of difference signal for higher sub-band */
	/* quanth: in-place for speed params: eh, deth (has init. value) */
	if eh >= 0 {
		ih = 3 /* 2,3 are pos codes */
	} else {
		ih = 1 /* 0,1 are neg codes */
	}
	decis = int32((564 * int64(deth)) >> 12)
	if abs(eh) > decis {
		ih-- /* mih = 2 case */
	}

	/* compute the quantized difference signal, higher sub-band*/
	dh = int32((int64(deth) * int64(qq2_code2_table[ih])) >> 15)

	/* logsch: update logarithmic quantizer scale factor in hi sub-band*/
	nbh = logsch(ih, nbh)

	/* note : scalel and scaleh use same code, different parameters */
	deth = scalel(nbh, 10)

	/* parrec - add pole predictor output to quantized diff. signal */
	ph = dh + szh

	/* upzero: update zero section predictor coefficients (sixth order) */
	/* calling parameters: dh, dhi, bphi */
	/* return params: updated bphi, delayed dhx */
	upzero(dh, delay_dhx[:], delay_bph[:])

	/* uppol2: update second predictor coef aph2 and delay as ah2 */
	/* calling params: ah1, ah2, ph, ph1, ph2 */
	ah2 = uppol2(ah1, ah2, ph, ph1, ph2)

	/* uppol1:  update first predictor coef. aph2 and delay it as ah1 */
	ah1 = uppol1(ah1, ah2, ph, ph1)

	/* recons for higher sub-band */
	yh = sh + dh

	/* done with higher sub-band encoder, now Delay for next time */
	rh2 = rh1
	rh1 = yh
	ph2 = ph1
	ph1 = ph

	/* multiplex ih and il to get signals together */
	return (il | (ih << 6))
}

/* decode function, result in xout1 and xout2 */
func decode(input int32) {
	var i int
	var xa1, xa2 int64
	var h_ptr int
	var ac_ptr, ac_ptr1, ad_ptr, ad_ptr1 int

	/* split transmitted word from input into ilr and ih */
	ilr = input & 0x3f
	ih = input >> 6

	/* LOWER SUB_BAND DECODER */

	/* filtez: compute predictor output for zero section */
	dec_szl = filtez(dec_del_bpl[:], dec_del_dltx[:])

	/* filtep: compute predictor output signal for pole section */
	dec_spl = filtep(dec_rlt1, dec_al1, dec_rlt2, dec_al2)

	dec_sl = dec_spl + dec_szl

	/* compute quantized difference signal for adaptive predic */
	dec_dlt = int32((int64(dec_detl) * int64(qq4_code4_table[ilr>>2])) >> 15)

	/* compute quantized difference signal for decoder output */
	dl = int32((int64(dec_detl) * int64(qq6_code6_table[il])) >> 15)

	rl = dl + dec_sl

	/* logscl: quantizer scale factor adaptation in the lower sub-band */
	dec_nbl = logscl(ilr, dec_nbl)

	/* scalel: computes quantizer scale factor in the lower sub band */
	dec_detl = scalel(dec_nbl, 8)

	/* parrec - add pole predictor output to quantized diff. signal */
	/* for partially reconstructed signal */
	dec_plt = dec_dlt + dec_szl

	/* upzero: update zero section predictor coefficients */
	upzero(dec_dlt, dec_del_dltx[:], dec_del_bpl[:])

	/* uppol2: update second predictor coefficient apl2 and delay it as al2 */
	dec_al2 = uppol2(dec_al1, dec_al2, dec_plt, dec_plt1, dec_plt2)

	/* uppol1: update first predictor coef. (pole setion) */
	dec_al1 = uppol1(dec_al1, dec_al2, dec_plt, dec_plt1)

	/* recons : compute recontructed signal for adaptive predictor */
	dec_rlt = dec_sl + dec_dlt

	/* done with lower sub band decoder, implement delays for next time */
	dec_rlt2 = dec_rlt1
	dec_rlt1 = dec_rlt
	dec_plt2 = dec_plt1
	dec_plt1 = dec_plt

	/* HIGH SUB-BAND DECODER */

	/* filtez: compute predictor output for zero section */
	dec_szh = filtez(dec_del_bph[:], dec_del_dhx[:])

	/* filtep: compute predictor output signal for pole section */
	dec_sph = filtep(dec_rh1, dec_ah1, dec_rh2, dec_ah2)

	/* predic:compute the predictor output value in the higher sub_band decoder */
	dec_sh = dec_sph + dec_szh

	/* in-place compute the quantized difference signal */
	dec_dh = int32((int64(dec_deth) * int64(qq2_code2_table[ih])) >> 15)

	/* logsch: update logarithmic quantizer scale factor in hi sub band */
	dec_nbh = logsch(ih, dec_nbh)

	/* scalel: compute the quantizer scale factor in the higher sub band */
	dec_deth = scalel(dec_nbh, 10)

	/* parrec: compute partially recontructed signal */
	dec_ph = dec_dh + dec_szh

	/* upzero: update zero section predictor coefficients */
	upzero(dec_dh, dec_del_dhx[:], dec_del_bph[:])

	/* uppol2: update second predictor coefficient aph2 and delay it as ah2 */
	dec_ah2 = uppol2(dec_ah1, dec_ah2, dec_ph, dec_ph1, dec_ph2)

	/* uppol1: update first predictor coef. (pole setion) */
	dec_ah1 = uppol1(dec_ah1, dec_ah2, dec_ph, dec_ph1)

	/* recons : compute recontructed signal for adaptive predictor */
	rh = dec_sh + dec_dh

	/* done with high band decode, implementing delays for next time here */
	dec_rh2 = dec_rh1
	dec_rh1 = rh
	dec_ph2 = dec_ph1
	dec_ph1 = dec_ph

	/* end of higher sub_band decoder */

	/* end with receive quadrature mirror filters */
	xd = rl - rh
	xs = rl + rh

	/* receive quadrature mirror filters implemented here */
	h_ptr = 0
	ac_ptr = 0
	ad_ptr = 0
	xa1 = int64(xd) * int64(h[h_ptr])
	h_ptr++
	xa2 = int64(xs) * int64(h[h_ptr])
	h_ptr++

	/* main multiply accumulate loop for samples and coefficients */
	for i = 0; i < 10; i++ {
		xa1 += int64(accumc[ac_ptr]) * int64(h[h_ptr])
		ac_ptr++
		h_ptr++
		xa2 += int64(accumd[ad_ptr]) * int64(h[h_ptr])
		ad_ptr++
		h_ptr++
	}

	/* final mult/accumulate */
	xa1 += int64(accumc[ac_ptr]) * int64(h[h_ptr])
	h_ptr++
	xa2 += int64(accumd[ad_ptr]) * int64(h[h_ptr])
	h_ptr++

	/* scale by 2^14 */
	xout1 = int32(xa1 >> 14)
	xout2 = int32(xa2 >> 14)

	/* update delay lines */
	ac_ptr1 = ac_ptr - 1
	ad_ptr1 = ad_ptr - 1
	for i = 0; i < 10; i++ {
		accumc[ac_ptr] = accumc[ac_ptr1]
		ac_ptr--
		ac_ptr1--
		accumd[ad_ptr] = accumd[ad_ptr1]
		ad_ptr--
		ad_ptr1--
	}
	accumc[ac_ptr] = xd
	accumd[ad_ptr] = xs
}

/* clear all storage locations */
func reset() {
	detl = 32 /* reset to min scale factor */
	dec_detl = 32
	deth = 8
	dec_deth = 8
	nbl = 0
	al1 = 0
	al2 = 0
	plt1 = 0
	plt2 = 0
	rlt1 = 0
	rlt2 = 0
	nbh = 0
	ah1 = 0
	ah2 = 0
	ph1 = 0
	ph2 = 0
	rh1 = 0
	rh2 = 0
	dec_nbl = 0
	dec_al1 = 0
	dec_al2 = 0
	dec_plt1 = 0
	dec_plt2 = 0
	dec_rlt1 = 0
	dec_rlt2 = 0
	dec_nbh = 0
	dec_ah1 = 0
	dec_ah2 = 0
	dec_ph1 = 0
	dec_ph2 = 0
	dec_rh1 = 0
	dec_rh2 = 0

	for i := 0; i < 6; i++ {
		delay_dltx[i] = 0
		delay_dhx[i] = 0
		dec_del_dltx[i] = 0
		dec_del_dhx[i] = 0
	}

	for i := 0; i < 6; i++ {
		delay_bpl[i] = 0
		delay_bph[i] = 0
		dec_del_bpl[i] = 0
		dec_del_bph[i] = 0
	}

	for i := 0; i < 24; i++ {
		tqmf[i] = 0
	}

	for i := 0; i < 11; i++ {
		accumc[i] = 0
		accumd[i] = 0
	}
}

/* filtez - compute predictor output signal (zero section) */
/* input: bpl1-6 and dlt1-6, output: szl */
func filtez(bpl, dlt []int32) int32 {
	var zl int64
	zl = int64(bpl[0]) * int64(dlt[0])
	for i := 1; i < 6; i++ {
		zl += int64(bpl[i]) * int64(dlt[i])
	}
	return int32(zl >> 14) /* x2 here */
}

/* filtep - compute predictor output signal (pole section) */
/* input rlt1-2 and al1-2, output spl */
func filtep(rlt1, al1, rlt2, al2 int32) int32 {
	var pl, pl2 int64
	pl = 2 * int64(rlt1)
	pl = int64(al1) * pl
	pl2 = 2 * int64(rlt2)
	pl += int64(al2) * pl2
	return int32(pl >> 15)
}

/* quantl - quantize the difference signal in the lower sub-band */
func quantl(el, detl int32) int32 {
	var ril, mil int32
	var wd, decis int64

	/* abs of difference signal */
	wd = int64(abs(el))
	/* determine mil based on decision levels and detl gain */
	for mil = 0; mil < 30; mil++ {
		decis = (int64(decis_levl[mil]) * int64(detl)) >> 15
		if wd <= decis {
			break
		}
	}
	/* if mil=30 then wd is less than all decision levels */
	if el >= 0 {
		ril = quant26bt_pos[mil]
	} else {
		ril = quant26bt_neg[mil]
	}
	return ril
}

/* logscl - update log quantizer scale factor in lower sub-band */
/* note that nbl is passed and returned */
func logscl(il, nbl int32) int32 {
	var wd int64
	wd = (int64(nbl) * 127) >> 7 /* leak factor 127/128 */
	nbl = int32(wd) + wl_code_table[il>>2]
	if nbl < 0 {
		nbl = 0
	}
	if nbl > 18432 {
		nbl = 18432
	}
	return nbl
}

/* scalel: compute quantizer scale factor in lower or upper sub-band*/
/* 修改: shift_constant 从 int 改为 int32，解决类型不匹配错误 */
func scalel(nbl int32, shift_constant int32) int32 {
	var wd1, wd2, wd3 int32
	wd1 = (nbl >> 6) & 31
	wd2 = nbl >> 11
	wd3 = ilb_table[wd1] >> (shift_constant + 1 - wd2)
	return wd3 << 3
}

/* upzero - inputs: dlt, dlti[0-5], bli[0-5], outputs: updated bli[0-5] */
/* also implements delay of bli and update of dlti from dlt */
func upzero(dlt int32, dlti, bli []int32) {
	var wd2, wd3 int32
	/*if dlt is zero, then no sum into bli */
	if dlt == 0 {
		for i := 0; i < 6; i++ {
			bli[i] = int32((255 * int64(bli[i])) >> 8) /* leak factor of 255/256 */
		}
	} else {
		for i := 0; i < 6; i++ {
			if int64(dlt)*int64(dlti[i]) >= 0 {
				wd2 = 128
			} else {
				wd2 = -128
			}
			wd3 = int32((255 * int64(bli[i])) >> 8) /* leak factor of 255/256 */
			bli[i] = wd2 + wd3
		}
	}
	/* implement delay line for dlt */
	dlti[5] = dlti[4]
	dlti[4] = dlti[3]
	dlti[3] = dlti[2]
	dlti[2] = dlti[1]
	dlti[1] = dlti[0]
	dlti[0] = dlt
}

/* uppol2 - update second predictor coefficient (pole section) */
/* inputs: al1, al2, plt, plt1, plt2. outputs: apl2 */
func uppol2(al1, al2, plt, plt1, plt2 int32) int32 {
	var wd2, wd4 int64
	var apl2 int32
	wd2 = 4 * int64(al1)
	if int64(plt)*int64(plt1) >= 0 {
		wd2 = -wd2 /* check same sign */
	}
	wd2 = wd2 >> 7 /* gain of 1/128 */
	if int64(plt)*int64(plt2) >= 0 {
		wd4 = wd2 + 128 /* same sign case */
	} else {
		wd4 = wd2 - 128
	}
	apl2 = int32(wd4 + (127*int64(al2))>>7) /* leak factor of 127/128 */

	/* apl2 is limited to +-.75 */
	if apl2 > 12288 {
		apl2 = 12288
	}
	if apl2 < -12288 {
		apl2 = -12288
	}
	return apl2
}

/* uppol1 - update first predictor coefficient (pole section) */
/* inputs: al1, apl2, plt, plt1. outputs: apl1 */
func uppol1(al1, apl2, plt, plt1 int32) int32 {
	var wd2 int64
	var wd3, apl1 int32
	wd2 = (int64(al1) * 255) >> 8 /* leak factor of 255/256 */
	if int64(plt)*int64(plt1) >= 0 {
		apl1 = int32(wd2) + 192 /* same sign case */
	} else {
		apl1 = int32(wd2) - 192
	}
	/* note: wd3= .9375-.75 is always positive */
	wd3 = 15360 - apl2 /* limit value */
	if apl1 > wd3 {
		apl1 = wd3
	}
	if apl1 < -wd3 {
		apl1 = -wd3
	}
	return apl1
}

/* logsch - update log quantizer scale factor in higher sub-band */
/* note that nbh is passed and returned */
func logsch(ih, nbh int32) int32 {
	var wd int64
	wd = (int64(nbh) * 127) >> 7 /* leak factor 127/128 */
	nbh = int32(wd) + wh_code_table[ih]
	if nbh < 0 {
		nbh = 0
	}
	if nbh > 22528 {
		nbh = 22528
	}
	return nbh
}

/*
+--------------------------------------------------------------------------+
| * Test Vectors (added for CHStone)                                       |
|     test_data : input data                                               |
|     test_compressed : expected output data for "encode"                  |
|     test_result : expected output data for "decode"                      |
+--------------------------------------------------------------------------+
*/

const SIZE = 100
const IN_END = 100

var test_data = [SIZE]int32{
	0x44, 0x44, 0x44, 0x44, 0x44,
	0x44, 0x44, 0x44, 0x44, 0x44,
	0x44, 0x44, 0x44, 0x44, 0x44,
	0x44, 0x44, 0x43, 0x43, 0x43,
	0x43, 0x43, 0x43, 0x43, 0x42,
	0x42, 0x42, 0x42, 0x42, 0x42,
	0x41, 0x41, 0x41, 0x41, 0x41,
	0x40, 0x40, 0x40, 0x40, 0x40,
	0x40, 0x40, 0x40, 0x3f, 0x3f,
	0x3f, 0x3f, 0x3f, 0x3e, 0x3e,
	0x3e, 0x3e, 0x3e, 0x3e, 0x3d,
	0x3d, 0x3d, 0x3d, 0x3d, 0x3d,
	0x3c, 0x3c, 0x3c, 0x3c, 0x3c,
	0x3c, 0x3c, 0x3c, 0x3c, 0x3b,
	0x3b, 0x3b, 0x3b, 0x3b, 0x3b,
	0x3b, 0x3b, 0x3b, 0x3b, 0x3b,
	0x3b, 0x3b, 0x3b, 0x3b, 0x3b,
	0x3b, 0x3b, 0x3b, 0x3b, 0x3b,
	0x3b, 0x3b, 0x3c, 0x3c, 0x3c,
	0x3c, 0x3c, 0x3c, 0x3c, 0x3c,
}

var compressed [SIZE]int32
var result [SIZE]int32

var test_compressed = [SIZE]int32{
	0xfd, 0xde, 0x77, 0xba, 0xf2,
	0x90, 0x20, 0xa0, 0xec, 0xed,
	0xef, 0xf1, 0xf3, 0xf4, 0xf5,
	0xf5, 0xf5, 0xf5, 0xf6, 0xf6,
	0xf6, 0xf7, 0xf8, 0xf7, 0xf8,
	0xf7, 0xf9, 0xf8, 0xf7, 0xf9,
	0xf8, 0xf8, 0xf6, 0xf8, 0xf8,
	0xf7, 0xf9, 0xf9, 0xf9, 0xf8,
	0xf7, 0xfa, 0xf8, 0xf8, 0xf7,
	0xfb, 0xfa, 0xf9, 0xf8, 0xf8,
}

var test_result = [SIZE]int32{
	0, -1, -1, 0, 0,
	-1, 0, 0, -1, -1,
	0, 0, 0x1, 0x1, 0,
	-2, -1, -2, 0, -4,
	0x1, 0x1, 0x1, -5, 0x2,
	0x2, 0x3, 0xb, 0x14, 0x14,
	0x16, 0x18, 0x20, 0x21, 0x26,
	0x27, 0x2e, 0x2f, 0x33, 0x32,
	0x35, 0x33, 0x36, 0x34, 0x37,
	0x34, 0x37, 0x35, 0x38, 0x36,
	0x39, 0x38, 0x3b, 0x3a, 0x3f,
	0x3f, 0x40, 0x3a, 0x3d, 0x3e,
	0x41, 0x3c, 0x3e, 0x3f, 0x42,
	0x3e, 0x3b, 0x37, 0x3b, 0x3e,
	0x41, 0x3b, 0x3b, 0x3a, 0x3b,
	0x36, 0x39, 0x3b, 0x3f, 0x3c,
	0x3b, 0x37, 0x3b, 0x3d, 0x41,
	0x3d, 0x3e, 0x3c, 0x3e, 0x3b,
	0x3a, 0x37, 0x3b, 0x3e, 0x41,
	0x3c, 0x3b, 0x39, 0x3a, 0x36,
}

func adpcm_main() {
	/* reset, initialize required memory */
	reset()

	for i := 0; i < IN_END; i += 2 {
		compressed[i/2] = encode(test_data[i], test_data[i+1])
	}
	for i := 0; i < IN_END; i += 2 {
		decode(compressed[i / 2])
		result[i] = xout1
		result[i+1] = xout2
	}
}

func main() {
	var main_result int32 = 0
	adpcm_main()
	for i := 0; i < IN_END/2; i++ {
		if compressed[i] != test_compressed[i] {
			main_result += 1
		}
	}
	for i := 0; i < IN_END; i++ {
		if result[i] != test_result[i] {
			main_result += 1
		}
	}
	fmt.Printf("%d\n", main_result)
}
