package main

import (
	"fmt"
	"math"
)

/*
----------------------------------------------------------------------------
| 类型定义
*----------------------------------------------------------------------------
*/
type flag bool
type int8_t int8
type int16_t int16
type int32_t int32
type uint32_t uint32
type bits16 uint16
type bits32 uint32
type bits64 uint64
type sbits64 int64

type float64_t uint64

/*
----------------------------------------------------------------------------
| 常量定义
*----------------------------------------------------------------------------
*/
const (
	float_tininess_after_rounding  int8_t = 0
	float_tininess_before_rounding int8_t = 1

	float_round_nearest_even int8_t = 0
	float_round_to_zero      int8_t = 1
	float_round_up           int8_t = 2
	float_round_down         int8_t = 3

	float_flag_inexact   int8_t = 1
	float_flag_divbyzero int8_t = 2
	float_flag_underflow int8_t = 4
	float_flag_overflow  int8_t = 8
	float_flag_invalid   int8_t = 16
)

/*
----------------------------------------------------------------------------
| 全局变量
*----------------------------------------------------------------------------
*/
var float_rounding_mode int8_t = float_round_nearest_even
var float_exception_flags int8_t = 0
var float_detect_tininess int8_t = float_tininess_before_rounding

/*
----------------------------------------------------------------------------
| 辅助函数
*----------------------------------------------------------------------------
*/
func boolToUint64(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

func shift64RightJamming(a bits64, count int16_t) bits64 {
	if count == 0 {
		return a
	} else if count < 64 {
		return (a >> uint(count)) | bits64(boolToUint64((a<<uint((-count)&63)) != 0))
	} else {
		return bits64(boolToUint64(a != 0))
	}
}

func shift64ExtraRightJamming(a0, a1 bits64, count int16_t) (z0, z1 bits64) {
	negCount := int8_t((-count) & 63)

	if count == 0 {
		return a0, a1
	} else if count < 64 {
		z1 = (a0 << uint(negCount)) | bits64(boolToUint64(a1 != 0))
		z0 = a0 >> uint(count)
		return
	} else {
		if count == 64 {
			z1 = a0 | bits64(boolToUint64(a1 != 0))
		} else {
			z1 = bits64(boolToUint64((a0 | a1) != 0))
		}
		z0 = 0
		return
	}
}

func add128(a0, a1, b0, b1 bits64) (z0, z1 bits64) {
	z1 = a1 + b1
	z0 = a0 + b0
	if z1 < a1 {
		z0++
	}
	return
}

func sub128(a0, a1, b0, b1 bits64) (z0, z1 bits64) {
	z1 = a1 - b1
	z0 = a0 - b0
	if a1 < b1 {
		z0--
	}
	return
}

func mul64To128(a, b bits64) (z0, z1 bits64) {
	aLow := bits32(a)
	aHigh := bits32(a >> 32)
	bLow := bits32(b)
	bHigh := bits32(b >> 32)

	z1 = bits64(aLow) * bits64(bLow)
	zMiddleA := bits64(aLow) * bits64(bHigh)
	zMiddleB := bits64(aHigh) * bits64(bLow)
	z0 = bits64(aHigh) * bits64(bHigh)

	zMiddleA += zMiddleB
	z0 += bits64(boolToUint64(zMiddleA < zMiddleB)) << 32
	z0 += zMiddleA >> 32
	zMiddleA <<= 32
	z1 += zMiddleA
	if z1 < zMiddleA {
		z0++
	}
	return
}

func estimateDiv128To64(a0, a1, b bits64) bits64 {
	if b <= a0 {
		return 0xFFFFFFFFFFFFFFFF
	}
	b0 := b >> 32
	var z bits64
	if (b0 << 32) <= a0 {
		z = 0xFFFFFFFF00000000
	} else {
		z = (a0 / b0) << 32
	}

	term0, term1 := mul64To128(b, z)
	rem0, rem1 := sub128(a0, a1, term0, term1)

	for sbits64(rem0) < 0 {
		z -= 0x100000000
		b1 := b << 32
		rem0, rem1 = add128(rem0, rem1, b0, b1)
	}
	rem0 = (rem0 << 32) | (rem1 >> 32)
	if (b0 << 32) <= rem0 {
		z |= 0xFFFFFFFF
	} else {
		z |= rem0 / b0
	}
	return z
}

func countLeadingZeros32(a bits32) int8_t {
	countLeadingZerosHigh := [256]int8_t{
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

	shiftCount := int8_t(0)
	if a < 0x10000 {
		shiftCount += 16
		a <<= 16
	}
	if a < 0x1000000 {
		shiftCount += 8
		a <<= 8
	}
	shiftCount += countLeadingZerosHigh[a>>24]
	return shiftCount
}

func countLeadingZeros64(a bits64) int8_t {
	shiftCount := int8_t(0)
	if a < (bits64(1) << 32) {
		shiftCount += 32
	} else {
		a >>= 32
	}
	shiftCount += countLeadingZeros32(bits32(a))
	return shiftCount
}

/*
----------------------------------------------------------------------------
| 特化函数
*----------------------------------------------------------------------------
*/
func float_raise(flags int8_t) {
	float_exception_flags |= flags
}

const float64_default_nan bits64 = 0x7FFFFFFFFFFFFFFF

func float64_is_nan(a float64_t) flag {
	return (bits64)(0xFFE0000000000000) < (bits64)((a << 1))
}

func float64_is_signaling_nan(a float64_t) flag {
	return (((a >> 51) & 0xFFF) == 0xFFE) && (a&0x0007FFFFFFFFFFFF) != 0
}

func propagateFloat64NaN(a, b float64_t) float64_t {
	aIsSignalingNaN := float64_is_signaling_nan(a)
	bIsNaN := float64_is_nan(b)
	bIsSignalingNaN := float64_is_signaling_nan(b)

	_ = float64_is_nan(a)

	a |= 0x0008000000000000
	b |= 0x0008000000000000

	if aIsSignalingNaN || bIsSignalingNaN {
		float_raise(float_flag_invalid)
	}

	if bIsSignalingNaN {
		return b
	} else if aIsSignalingNaN {
		return a
	} else if bIsNaN {
		return b
	} else {
		return a
	}
}

/*
----------------------------------------------------------------------------
| 核心浮点运算函数
*----------------------------------------------------------------------------
*/
func extractFloat64Frac(a float64_t) bits64 {
	return bits64(a & 0x000FFFFFFFFFFFFF)
}

func extractFloat64Exp(a float64_t) int16_t {
	return int16_t((a >> 52) & 0x7FF)
}

func extractFloat64Sign(a float64_t) flag {
	return a>>63 != 0
}

func normalizeFloat64Subnormal(aSig bits64) (int16_t, bits64) {
	shiftCount := countLeadingZeros64(aSig) - 11
	return int16_t(1) - int16_t(shiftCount), aSig << uint(shiftCount)
}

func packFloat64(zSign flag, zExp int16_t, zSig bits64) float64_t {
	signBit := bits64(0)
	if zSign {
		signBit = 1
	}
	return float64_t((signBit << 63) + (bits64(zExp) << 52) + zSig)
}

// 关键修复：严格按照C代码逻辑重写
func roundAndPackFloat64(zSign flag, zExp int16_t, zSig bits64) float64_t {
	roundingMode := float_rounding_mode
	roundNearestEven := (roundingMode == float_round_nearest_even)
	roundIncrement := int16_t(0x200)

	if !roundNearestEven {
		if roundingMode == float_round_to_zero {
			roundIncrement = 0
		} else {
			roundIncrement = 0x3FF
			if zSign {
				if roundingMode == float_round_up {
					roundIncrement = 0
				}
			} else {
				if roundingMode == float_round_down {
					roundIncrement = 0
				}
			}
		}
	}

	roundBits := int16_t(zSig & 0x3FF)
	if 0x7FD <= uint16(zExp) {
		if (0x7FD < zExp) || ((zExp == 0x7FD) && (sbits64(zSig+bits64(roundIncrement)) < 0)) {
			float_raise(float_flag_overflow | float_flag_inexact)
			if (roundIncrement == 0) || zSign {
				return packFloat64(zSign, 0x7FF, 0)
			}
			return packFloat64(zSign, 0x7FF, 0)
		}
		if zExp < 0 {
			isTiny := (float_detect_tininess == float_tininess_before_rounding) ||
				(zExp < -1) ||
				(zSig+bits64(roundIncrement) < 0x8000000000000000)
			zSig = shift64RightJamming(zSig, -zExp)
			zExp = 0
			roundBits = int16_t(zSig & 0x3FF)
			if isTiny && roundBits != 0 {
				float_raise(float_flag_underflow)
			}
		}
	}

	if roundBits != 0 {
		float_exception_flags |= float_flag_inexact
	}

	// 修复：正确处理舍入和偶数化
	zSig = (zSig + bits64(roundIncrement)) >> 10
	// 关键修复：当roundBits == 0x200且roundNearestEven时，清除最低位（偶数化）
	if (roundBits^0x200) == 0 && roundNearestEven {
		zSig &^= 1
	}

	if zSig == 0 {
		zExp = 0
	}
	return packFloat64(zSign, zExp, zSig)
}

func normalizeRoundAndPackFloat64(zSign flag, zExp int16_t, zSig bits64) float64_t {
	shiftCount := countLeadingZeros64(zSig) - 1
	return roundAndPackFloat64(zSign, zExp-int16_t(shiftCount), zSig<<uint(shiftCount))
}

func int32_to_float64(a int32_t) float64_t {
	if a == 0 {
		return 0
	}
	zSign := a < 0
	var absA uint32
	if zSign {
		absA = uint32(-a)
	} else {
		absA = uint32(a)
	}
	shiftCount := countLeadingZeros32(bits32(absA)) + 21
	zSig := bits64(absA)
	return packFloat64(flag(zSign), 0x432-int16_t(shiftCount), zSig<<uint(shiftCount))
}

func addFloat64Sigs(a, b float64_t, zSign flag) float64_t {
	aSig := extractFloat64Frac(a)
	aExp := extractFloat64Exp(a)
	bSig := extractFloat64Frac(b)
	bExp := extractFloat64Exp(b)

	expDiff := aExp - bExp
	aSig <<= 9
	bSig <<= 9

	if 0 < expDiff {
		if aExp == 0x7FF {
			if aSig != 0 {
				return propagateFloat64NaN(a, b)
			}
			return a
		}
		if bExp == 0 {
			expDiff--
		} else {
			bSig |= 0x2000000000000000
		}
		bSig = shift64RightJamming(bSig, expDiff)
	} else if expDiff < 0 {
		if bExp == 0x7FF {
			if bSig != 0 {
				return propagateFloat64NaN(a, b)
			}
			return packFloat64(zSign, 0x7FF, 0)
		}
		if aExp == 0 {
			expDiff++
		} else {
			aSig |= 0x2000000000000000
		}
		aSig = shift64RightJamming(aSig, -expDiff)
	} else {
		if aExp == 0x7FF {
			if aSig|bSig != 0 {
				return propagateFloat64NaN(a, b)
			}
			return a
		}
		if aExp == 0 {
			return packFloat64(zSign, 0, (aSig+bSig)>>9)
		}
		zSig := bits64(0x4000000000000000) + aSig + bSig
		zExp := aExp
		return roundAndPackFloat64(zSign, zExp, zSig)
	}

	aSig |= 0x2000000000000000
	zSig := (aSig + bSig) << 1
	zExp := aExp - 1
	if sbits64(zSig) < 0 {
		zSig = aSig + bSig
		zExp = aExp
	}
	return roundAndPackFloat64(zSign, zExp, zSig)
}

// 关键修复：将变量声明移到函数开头，避免 goto 跳过声明
func subFloat64Sigs(a, b float64_t, zSign flag) float64_t {
	aSig := extractFloat64Frac(a)
	aExp := extractFloat64Exp(a)
	bSig := extractFloat64Frac(b)
	bExp := extractFloat64Exp(b)

	expDiff := aExp - bExp
	aSig <<= 10
	bSig <<= 10

	// 关键：在所有标签之前声明变量，避免 goto 跳过声明
	var zSig bits64
	var zExp int16_t

	if 0 < expDiff {
		goto aExpBigger
	}
	if expDiff < 0 {
		goto bExpBigger
	}

	// expDiff == 0
	if aExp == 0x7FF {
		if aSig|bSig != 0 {
			return propagateFloat64NaN(a, b)
		}
		float_raise(float_flag_invalid)
		return float64_t(float64_default_nan)
	}
	if aExp == 0 {
		aExp = 1
		bExp = 1
	}
	if bSig < aSig {
		goto aBigger
	}
	if aSig < bSig {
		goto bBigger
	}
	return packFloat64(float_rounding_mode == float_round_down, 0, 0)

aExpBigger:
	if aExp == 0x7FF {
		if aSig != 0 {
			return propagateFloat64NaN(a, b)
		}
		return a
	}
	if bExp == 0 {
		expDiff--
	} else {
		bSig |= 0x4000000000000000
	}
	bSig = shift64RightJamming(bSig, expDiff)
	aSig |= 0x4000000000000000

aBigger:
	zSig = aSig - bSig
	zExp = aExp
	goto normalizeRoundAndPack

bExpBigger:
	if bExp == 0x7FF {
		if bSig != 0 {
			return propagateFloat64NaN(a, b)
		}
		return packFloat64(!zSign, 0x7FF, 0)
	}
	if aExp == 0 {
		expDiff++
	} else {
		aSig |= 0x4000000000000000
	}
	aSig = shift64RightJamming(aSig, -expDiff)
	bSig |= 0x4000000000000000

bBigger:
	zSig = bSig - aSig
	zExp = bExp
	zSign = !zSign

normalizeRoundAndPack:
	zExp--
	return normalizeRoundAndPackFloat64(zSign, zExp, zSig)
}

func float64_add(a, b float64_t) float64_t {
	aSign := extractFloat64Sign(a)
	bSign := extractFloat64Sign(b)

	if aSign == bSign {
		return addFloat64Sigs(a, b, aSign)
	} else {
		return subFloat64Sigs(a, b, aSign)
	}
}

func float64_mul(a, b float64_t) float64_t {
	aSign := extractFloat64Sign(a)
	bSign := extractFloat64Sign(b)
	zSign := flag(aSign != bSign)

	aSig := extractFloat64Frac(a)
	aExp := extractFloat64Exp(a)
	bSig := extractFloat64Frac(b)
	bExp := extractFloat64Exp(b)

	if aExp == 0x7FF {
		if aSig != 0 || ((bExp == 0x7FF) && bSig != 0) {
			return propagateFloat64NaN(a, b)
		}
		if (bExp | int16_t(bSig)) == 0 {
			float_raise(float_flag_invalid)
			return float64_t(float64_default_nan)
		}
		return packFloat64(zSign, 0x7FF, 0)
	}
	if bExp == 0x7FF {
		if bSig != 0 {
			return propagateFloat64NaN(a, b)
		}
		if (aExp | int16_t(aSig)) == 0 {
			float_raise(float_flag_invalid)
			return float64_t(float64_default_nan)
		}
		return packFloat64(zSign, 0x7FF, 0)
	}
	if aExp == 0 {
		if aSig == 0 {
			return packFloat64(zSign, 0, 0)
		}
		aExp, aSig = normalizeFloat64Subnormal(aSig)
	}
	if bExp == 0 {
		if bSig == 0 {
			return packFloat64(zSign, 0, 0)
		}
		bExp, bSig = normalizeFloat64Subnormal(bSig)
	}
	zExp := aExp + bExp - 0x3FF
	aSig = (aSig | 0x0010000000000000) << 10
	bSig = (bSig | 0x0010000000000000) << 11

	zSig0, zSig1 := mul64To128(aSig, bSig)
	zSig0 |= bits64(boolToUint64(zSig1 != 0))

	if 0 <= sbits64(zSig0<<1) {
		zSig0 <<= 1
		zExp--
	}

	return roundAndPackFloat64(zSign, zExp, zSig0)
}

func float64_div(a, b float64_t) float64_t {
	aSign := extractFloat64Sign(a)
	bSign := extractFloat64Sign(b)
	zSign := flag(aSign != bSign)

	aSig := extractFloat64Frac(a)
	aExp := extractFloat64Exp(a)
	bSig := extractFloat64Frac(b)
	bExp := extractFloat64Exp(b)

	if aExp == 0x7FF {
		if aSig != 0 {
			return propagateFloat64NaN(a, b)
		}
		if bExp == 0x7FF {
			if bSig != 0 {
				return propagateFloat64NaN(a, b)
			}
			float_raise(float_flag_invalid)
			return float64_t(float64_default_nan)
		}
		return packFloat64(zSign, 0x7FF, 0)
	}
	if bExp == 0x7FF {
		if bSig != 0 {
			return propagateFloat64NaN(a, b)
		}
		return packFloat64(zSign, 0, 0)
	}
	if bExp == 0 {
		if bSig == 0 {
			if (aExp | int16_t(aSig)) == 0 {
				float_raise(float_flag_invalid)
				return float64_t(float64_default_nan)
			}
			float_raise(float_flag_divbyzero)
			return packFloat64(zSign, 0x7FF, 0)
		}
		bExp, bSig = normalizeFloat64Subnormal(bSig)
	}
	if aExp == 0 {
		if aSig == 0 {
			return packFloat64(zSign, 0, 0)
		}
		aExp, aSig = normalizeFloat64Subnormal(aSig)
	}
	zExp := aExp - bExp + 0x3FD
	aSig = (aSig | 0x0010000000000000) << 10
	bSig = (bSig | 0x0010000000000000) << 11
	if bSig <= (aSig + aSig) {
		aSig >>= 1
		zExp++
	}
	zSig := estimateDiv128To64(aSig, 0, bSig)
	if (zSig & 0x1FF) <= 2 {
		term0, term1 := mul64To128(bSig, zSig)
		rem0, rem1 := sub128(aSig, 0, term0, term1)
		for sbits64(rem0) < 0 {
			zSig--
			rem0, rem1 = add128(rem0, rem1, 0, bSig)
		}
		zSig |= bits64(boolToUint64(rem1 != 0))
	}
	return roundAndPackFloat64(zSign, zExp, zSig)
}

func float64_le(a, b float64_t) flag {
	if ((extractFloat64Exp(a) == 0x7FF) && extractFloat64Frac(a) != 0) ||
		((extractFloat64Exp(b) == 0x7FF) && extractFloat64Frac(b) != 0) {
		float_raise(float_flag_invalid)
		return false
	}
	aSign := extractFloat64Sign(a)
	bSign := extractFloat64Sign(b)

	if aSign != bSign {
		return aSign || ((a|b)<<1 == 0)
	}
	return (a == b) || (aSign != (a < b))
}

func float64_ge(a, b float64_t) flag {
	return float64_le(b, a)
}

func float64_neg(x float64_t) float64_t {
	return float64_t(((^x) & 0x8000000000000000) | (x & 0x7fffffffffffffff))
}

/*
----------------------------------------------------------------------------
| dfsin.c 主程序部分
*----------------------------------------------------------------------------
*/
func float64_abs(x float64_t) float64_t {
	return x & 0x7fffffffffffffff
}

func local_sin(rad float64_t) float64_t {
	x := math.Float64frombits(uint64(rad))
	app := x
	diff := x
	inc := 1
	m_rad2 := -x * x
	threshold := math.Float64frombits(0x3ee4f8b588e368f1)

	for {
		diff = diff * m_rad2 / float64((2*inc)*(2*inc+1))
		app += diff
		inc++
		if math.Abs(diff) < threshold {
			break
		}
	}
	return float64_t(math.Float64bits(app))
}

func ullong_to_double(x uint64) float64 {
	return math.Float64frombits(x)
}

const N = 36

var test_in = [N]float64_t{
	0x0000000000000000, /*      0  */
	0x3fc65717fced55c1, /*   PI/18 */
	0x3fd65717fced55c1, /*   PI/9  */
	0x3fe0c151fdb20051, /*   PI/6  */
	0x3fe65717fced55c1, /*  2PI/9  */
	0x3febecddfc28ab31, /*  5PI/18 */
	0x3ff0c151fdb20051, /*   PI/3  */
	0x3ff38c34fd4fab09, /*  7PI/18 */
	0x3ff65717fced55c1, /*  4PI/9  */
	0x3ff921fafc8b0079, /*   PI/2  */
	0x3ffbecddfc28ab31, /*  5PI/9  */
	0x3ffeb7c0fbc655e9, /* 11PI/18 */
	0x4000c151fdb20051, /*  2PI/3  */
	0x400226c37d80d5ad, /* 13PI/18 */
	0x40038c34fd4fab09, /*  7PI/9  */
	0x4004f1a67d1e8065, /*  5PI/6  */
	0x40065717fced55c1, /*  8PI/9  */
	0x4007bc897cbc2b1d, /* 17PI/18 */
	0x400921fafc8b0079, /*   PI    */
	0x400a876c7c59d5d5, /* 19PI/18 */
	0x400becddfc28ab31, /* 10PI/9  */
	0x400d524f7bf7808d, /*  7PI/6  */
	0x400eb7c0fbc655e9, /* 11PI/9  */
	0x40100e993dca95a3, /* 23PI/18 */
	0x4010c151fdb20051, /*  8PI/6  */
	0x4011740abd996aff, /* 25PI/18 */
	0x401226c37d80d5ad, /* 13PI/9  */
	0x4012d97c3d68405b, /*  3PI/2  */
	0x40138c34fd4fab09, /* 14PI/9  */
	0x40143eedbd3715b7, /* 29PI/18 */
	0x4014f1a67d1e8065, /* 15PI/9  */
	0x4015a45f3d05eb13, /* 31PI/18 */
	0x40165717fced55c1, /* 16PI/9  */
	0x401709d0bcd4c06f, /* 33PI/18 */
	0x4017bc897cbc2b1d, /* 17PI/9  */
	0x40186f423ca395cb, /* 35PI/18 */
}

var test_out = [N]float64_t{
	0x0000000000000000, /*  0.000000 */
	0x3fc63a1a335aadcd, /*  0.173648 */
	0x3fd5e3a82b09bf3e, /*  0.342020 */
	0x3fdfffff91f9aa91, /*  0.500000 */
	0x3fe491b716c242e3, /*  0.642787 */
	0x3fe8836f672614a6, /*  0.766044 */
	0x3febb67ac40b2bed, /*  0.866025 */
	0x3fee11f6127e28ad, /*  0.939693 */
	0x3fef838b6adffac0, /*  0.984808 */
	0x3fefffffe1cbd7aa, /*  1.000000 */
	0x3fef838bb0147989, /*  0.984808 */
	0x3fee11f692d962b4, /*  0.939693 */
	0x3febb67b77c0142d, /*  0.866026 */
	0x3fe883709d4ea869, /*  0.766045 */
	0x3fe491b81d72d8e8, /*  0.642788 */
	0x3fe00000ea5f43c8, /*  0.500000 */
	0x3fd5e3aa4e0590c5, /*  0.342021 */
	0x3fc63a1d2189552c, /*  0.173648 */
	0x3ea6aedffc454b91, /*  0.000001 */
	0xbfc63a1444ddb37c, /* -0.173647 */
	0xbfd5e3a4e68f8f3e, /* -0.342019 */
	0xbfdffffd494cf96b, /* -0.499999 */
	0xbfe491b61cb9a3d3, /* -0.642787 */
	0xbfe8836eb2dcf815, /* -0.766044 */
	0xbfebb67a740aae32, /* -0.866025 */
	0xbfee11f5912d2157, /* -0.939692 */
	0xbfef838b1ac64afc, /* -0.984808 */
	0xbfefffffc2e5dc8f, /* -1.000000 */
	0xbfef838b5ea2e7ea, /* -0.984808 */
	0xbfee11f7112dae27, /* -0.939693 */
	0xbfebb67c2c31cb4a, /* -0.866026 */
	0xbfe883716e6fd781, /* -0.766045 */
	0xbfe491b9cd1b5d56, /* -0.642789 */
	0xbfe000021d0ca30d, /* -0.500001 */
	0xbfd5e3ad0a69caf7, /* -0.342021 */
	0xbfc63a23c48863dd, /* -0.173649 */
}

func main() {
	main_result := 0
	for i := 0; i < N; i++ {
		result := local_sin(test_in[i])
		if result != test_out[i] {
			main_result++
		}
		fmt.Printf("input=%016x expected=%016x output=%016x (%f)\n",
			uint64(test_in[i]), uint64(test_out[i]), uint64(result), ullong_to_double(uint64(result)))
	}
	fmt.Printf("%d\n", main_result)
}
