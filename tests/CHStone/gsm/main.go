package main

import (
	"fmt"
	"os"
)

// 类型定义 (来自 private.h)
type word int16     // 16位有符号整数
type longword int32 // 32位有符号整数

const (
	MIN_WORD = (-32767) - 1
	MAX_WORD = 32767
)

// 算术右移 (SASR)
func SASR(x longword, by uint) longword {
	return x >> by
}

// 饱和函数 (来自 add.c 的宏)
func saturate(x longword) word {
	if x < MIN_WORD {
		return MIN_WORD
	}
	if x > MAX_WORD {
		return MAX_WORD
	}
	return word(x)
}

// 基础运算函数 (来自 add.c)
func gsm_add(a, b word) word {
	sum := longword(a) + longword(b)
	return saturate(sum)
}

func gsm_mult(a, b word) word {
	if a == MIN_WORD && b == MIN_WORD {
		return MAX_WORD
	}
	return word(SASR(longword(a)*longword(b), 15))
}

func gsm_mult_r(a, b word) word {
	if b == MIN_WORD && a == MIN_WORD {
		return MAX_WORD
	}
	prod := longword(a)*longword(b) + 16384
	prod >>= 15
	return word(prod & 0xFFFF)
}

func gsm_abs(a word) word {
	if a < 0 {
		if a == MIN_WORD {
			return MAX_WORD
		}
		return -a
	}
	return a
}

// bitoff 查找表 (来自 add.c)
var bitoff = [256]byte{
	8, 7, 6, 6, 5, 5, 5, 5, 4, 4, 4, 4, 4, 4, 4, 4,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
	2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
}
// 归一化函数 (来自 add.c)
func gsm_norm(a longword) word {
	if a < 0 {
		if a <= -1073741824 {
			return 0
		}
		a = ^a // 按位取反
	}

	// 修复：使用 ^longword(0xFFFF) 代替 0xffff0000，使用 ^longword(0xFFFFFF) 代替 0xff000000
	if a&(^longword(0xFFFF)) != 0 {
		if a&(^longword(0xFFFFFF)) != 0 {
			return word(-1 + int16(bitoff[0xFF&(a>>24)]))
		}
		return word(7 + int16(bitoff[0xFF&(a>>16)]))
	}
	if a&longword(0xff00) != 0 {
		return word(15 + int16(bitoff[0xFF&(a>>8)]))
	}
	return word(23 + int16(bitoff[0xFF&a]))
}

// 除法函数 (来自 add.c)
func gsm_div(num, denum word) word {
	if num == 0 {
		return 0
	}

	L_num := longword(num)
	L_denum := longword(denum)
	div := word(0)

	for k := 0; k < 15; k++ {
		div <<= 1
		L_num <<= 1

		if L_num >= L_denum {
			L_num -= L_denum
			div++
		}
	}

	return div
}

// LPC 分析函数 (来自 lpc.c)

func Autocorrelation(s []word, L_ACF []longword) {
	var smax, temp, scalauto, n word

	// 寻找最大值
	smax = 0
	for k := 0; k <= 159; k++ {
		temp = gsm_abs(s[k])
		if temp > smax {
			smax = temp
		}
	}

	// 计算缩放因子
	if smax == 0 {
		scalauto = 0
	} else {
		scalauto = 4 - gsm_norm(longword(smax)<<16)
	}

	if scalauto > 0 && scalauto <= 4 {
		n = scalauto
		for k := 0; k <= 159; k++ {
			s[k] = gsm_mult_r(s[k], word(16384>>(n-1)))
		}
	}

	// 初始化 L_ACF
	for k := 0; k <= 8; k++ {
		L_ACF[k] = 0
	}

	// 手动展开累加循环 (对应C语言的宏展开)
	sp := 0
	sl := s[sp]

	// i=0
	L_ACF[0] += longword(sl) * longword(s[sp])

	// i=1
	sp++
	sl = s[sp]
	L_ACF[0] += longword(sl) * longword(s[sp])
	L_ACF[1] += longword(sl) * longword(s[sp-1])

	// i=2
	sp++
	sl = s[sp]
	L_ACF[0] += longword(sl) * longword(s[sp])
	L_ACF[1] += longword(sl) * longword(s[sp-1])
	L_ACF[2] += longword(sl) * longword(s[sp-2])

	// i=3
	sp++
	sl = s[sp]
	L_ACF[0] += longword(sl) * longword(s[sp])
	L_ACF[1] += longword(sl) * longword(s[sp-1])
	L_ACF[2] += longword(sl) * longword(s[sp-2])
	L_ACF[3] += longword(sl) * longword(s[sp-3])

	// i=4
	sp++
	sl = s[sp]
	L_ACF[0] += longword(sl) * longword(s[sp])
	L_ACF[1] += longword(sl) * longword(s[sp-1])
	L_ACF[2] += longword(sl) * longword(s[sp-2])
	L_ACF[3] += longword(sl) * longword(s[sp-3])
	L_ACF[4] += longword(sl) * longword(s[sp-4])

	// i=5
	sp++
	sl = s[sp]
	L_ACF[0] += longword(sl) * longword(s[sp])
	L_ACF[1] += longword(sl) * longword(s[sp-1])
	L_ACF[2] += longword(sl) * longword(s[sp-2])
	L_ACF[3] += longword(sl) * longword(s[sp-3])
	L_ACF[4] += longword(sl) * longword(s[sp-4])
	L_ACF[5] += longword(sl) * longword(s[sp-5])

	// i=6
	sp++
	sl = s[sp]
	L_ACF[0] += longword(sl) * longword(s[sp])
	L_ACF[1] += longword(sl) * longword(s[sp-1])
	L_ACF[2] += longword(sl) * longword(s[sp-2])
	L_ACF[3] += longword(sl) * longword(s[sp-3])
	L_ACF[4] += longword(sl) * longword(s[sp-4])
	L_ACF[5] += longword(sl) * longword(s[sp-5])
	L_ACF[6] += longword(sl) * longword(s[sp-6])

	// i=7
	sp++
	sl = s[sp]
	L_ACF[0] += longword(sl) * longword(s[sp])
	L_ACF[1] += longword(sl) * longword(s[sp-1])
	L_ACF[2] += longword(sl) * longword(s[sp-2])
	L_ACF[3] += longword(sl) * longword(s[sp-3])
	L_ACF[4] += longword(sl) * longword(s[sp-4])
	L_ACF[5] += longword(sl) * longword(s[sp-5])
	L_ACF[6] += longword(sl) * longword(s[sp-6])
	L_ACF[7] += longword(sl) * longword(s[sp-7])

	// i=8 到 159 的循环
	for i := 8; i <= 159; i++ {
		sp++
		sl = s[sp]
		L_ACF[0] += longword(sl) * longword(s[sp-0])
		L_ACF[1] += longword(sl) * longword(s[sp-1])
		L_ACF[2] += longword(sl) * longword(s[sp-2])
		L_ACF[3] += longword(sl) * longword(s[sp-3])
		L_ACF[4] += longword(sl) * longword(s[sp-4])
		L_ACF[5] += longword(sl) * longword(s[sp-5])
		L_ACF[6] += longword(sl) * longword(s[sp-6])
		L_ACF[7] += longword(sl) * longword(s[sp-7])
		L_ACF[8] += longword(sl) * longword(s[sp-8])
	}

	// 左移1位
	for k := 0; k <= 8; k++ {
		L_ACF[k] <<= 1
	}

	// 重新缩放数组 s
	if scalauto > 0 {
		for k := 159; k >= 0; k-- {
			s[k] <<= scalauto
		}
	}
}

func Reflection_coefficients(L_ACF []longword, r []word) {
	var ACF [9]word
	var P [9]word
	var K [9]word

	if L_ACF[0] == 0 {
		for i := 0; i < 8; i++ {
			r[i] = 0
		}
		return
	}

	temp := gsm_norm(L_ACF[0])
	for i := 0; i <= 8; i++ {
		ACF[i] = word(SASR(L_ACF[i]<<uint(temp), 16))
	}

	for i := 1; i <= 7; i++ {
		K[i] = ACF[i]
	}
	for i := 0; i <= 8; i++ {
		P[i] = ACF[i]
	}

	r_idx := 0
	for n := 1; n <= 8; n++ {
		temp = P[1]
		if temp < 0 {
			temp = -temp
		}

		if P[0] < temp {
			for i := n; i <= 8; i++ {
				r[r_idx] = 0
				r_idx++
			}
			return
		}

		r[r_idx] = gsm_div(temp, P[0])

		if P[1] > 0 {
			r[r_idx] = -r[r_idx]
		}

		if n == 8 {
			return
		}

		temp = gsm_mult_r(P[1], r[r_idx])
		P[0] = gsm_add(P[0], temp)

		for m := 1; m <= 8-n; m++ {
			temp = gsm_mult_r(K[m], r[r_idx])
			P[m] = gsm_add(P[m+1], temp)

			temp = gsm_mult_r(P[m+1], r[r_idx])
			K[m] = gsm_add(K[m], temp)
		}

		r_idx++
	}
}

func Transformation_to_Log_Area_Ratios(r []word) {
	for i := 0; i < 8; i++ {
		temp := r[i]
		if temp < 0 {
			temp = -temp
		}

		if temp < 22118 {
			temp >>= 1
		} else if temp < 31130 {
			temp -= 11059
		} else {
			temp -= 26112
			temp <<= 2
		}

		if r[i] < 0 {
			r[i] = -temp
		} else {
			r[i] = temp
		}
	}
}

func Quantization_and_coding(LAR []word) {
	type params struct {
		A, B     word
		MAC, MIC word
	}

	steps := []params{
		{20480, 0, 31, -32},
		{20480, 0, 31, -32},
		{20480, 2048, 15, -16},
		{20480, -2560, 15, -16},
		{13964, 94, 7, -8},
		{15360, -1792, 7, -8},
		{8534, -341, 3, -4},
		{9036, -1144, 3, -4},
	}

	for i := 0; i < 8; i++ {
		p := steps[i]
		temp := gsm_mult(p.A, LAR[i])
		temp = gsm_add(temp, p.B)
		temp = gsm_add(temp, 256)
		temp = word(SASR(longword(temp), 9))

		if temp > p.MAC {
			LAR[i] = p.MAC - p.MIC
		} else if temp < p.MIC {
			LAR[i] = 0
		} else {
			LAR[i] = temp - p.MIC
		}
	}
}

func Gsm_LPC_Analysis(s []word, LARc []word) {
	var L_ACF [9]longword
	Autocorrelation(s, L_ACF[:])
	Reflection_coefficients(L_ACF[:], LARc)
	Transformation_to_Log_Area_Ratios(LARc)
	Quantization_and_coding(LARc)
}

// 测试数据和主函数 (来自 gsm.c)
const (
	N = 160
	M = 8
)

var inData = [N]word{
	81, 10854, 1893, -10291, 7614, 29718, 20475, -29215, -18949, -29806,
	-32017, 1596, 15744, -3088, -17413, -22123, 6798, -13276, 3819, -16273,
	-1573, -12523, -27103, -193, -25588, 4698, -30436, 15264, -1393, 11418,
	11370, 4986, 7869, -1903, 9123, -31726, -25237, -14155, 17982, 32427,
	-12439, -15931, -21622, 7896, 1689, 28113, 3615, 22131, -5572, -20110,
	12387, 9177, -24544, 12480, 21546, -17842, -13645, 20277, 9987, 17652,
	-11464, -17326, -10552, -27100, 207, 27612, 2517, 7167, -29734, -22441,
	30039, -2368, 12813, 300, -25555, 9087, 29022, -6559, -20311, -14347,
	-7555, -21709, -3676, -30082, -3190, -30979, 8580, 27126, 3414, -4603,
	-22303, -17143, 13788, -1096, -14617, 22071, -13552, 32646, 16689, -8473,
	-12733, 10503, 20745, 6696, -26842, -31015, 3792, -19864, -20431, -30307,
	32421, -13237, 9006, 18249, 2403, -7996, -14827, -5860, 7122, 29817,
	-31894, 17955, 28836, -31297, 31821, -27502, 12276, -5587, -22105, 9192,
	-22549, 15675, -12265, 7212, -23749, -12856, -5857, 7521, 17349, 13773,
	-3091, -17812, -9655, 26667, 7902, 2487, 3177, 29412, -20224, -2776,
	24084, -7963, -10438, -11938, -14833, -6658, 32058, 4020, 10461, 15159,
}

var outData = [N]word{
	80, 10848, 1888, -10288, 7616, 29712, 20480, -29216, -18944, -29808,
	-32016, 1600, 15744, -3088, -17408, -22128, 6800, -13280, 3824, -16272,
	-1568, -12528, -27104, -192, -25584, 4704, -30432, 15264, -1392, 11424,
	11376, 4992, 7872, -1904, 9120, -31728, -25232, -14160, 17984, 32432,
	-12432, -15936, -21616, 7904, 1696, 28112, 3616, 22128, -5568, -20112,
	12384, 9184, -24544, 12480, 21552, -17840, -13648, 20272, 9984, 17648,
	-11456, -17328, -10544, -27104, 208, 27616, 2512, 7168, -29728, -22448,
	30032, -2368, 12816, 304, -25552, 9088, 29024, -6560, -20304, -14352,
	-7552, -21712, -3680, -30080, -3184, -30976, 8576, 27120, 3408, -4608,
	-22304, -17136, 13792, -1088, -14624, 22064, -13552, 32640, 16688, -8480,
	-12736, 10496, 20752, 6704, -26848, -31008, 3792, -19856, -20432, -30304,
	32416, -13232, 9008, 18256, 2400, -8000, -14832, -5856, 7120, 29824,
	-31888, 17952, 28832, -31296, 31824, -27504, 12272, -5584, -22112, 9200,
	-22544, 15680, -12272, 7216, -23744, -12848, -5856, 7520, 17344, 13776,
	-3088, -17808, -9648, 26672, 7904, 2480, 3184, 29408, -20224, -2768,
	24080, -7968, -10432, -11936, -14832, -6656, 32064, 4016, 10464, 15152,
}

var outLARc = [M]word{32, 33, 22, 13, 7, 5, 3, 2}

func main() {
	main_result := 0
	so := make([]word, N)
	LARc := make([]word, M)

	for i := 0; i < N; i++ {
		so[i] = inData[i]
	}

	Gsm_LPC_Analysis(so, LARc)

	for i := 0; i < N; i++ {
		if so[i] != outData[i] {
			main_result++
		}
	}
	for i := 0; i < M; i++ {
		if LARc[i] != outLARc[i] {
			main_result++
		}
	}

	fmt.Println(main_result)
	os.Exit(main_result)
}
