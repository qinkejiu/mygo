package main

import (
	"fmt"
	"math"
)

/*----------------------------------------------------------------------------
| Type definitions (from SPARC-GCC.h and softfloat.h)
*----------------------------------------------------------------------------*/
type flag int8
type int8t int8  // renamed to avoid conflict with builtin
type int16t int16 // renamed to avoid conflict with builtin
type bits16 uint16
type bits32 uint32
type bits64 uint64
type sbits64 int64

// Software IEC/IEEE floating-point types
type float32 uint32
type float64t uint64 // renamed to avoid conflict with builtin float64

/*----------------------------------------------------------------------------
| Global state
*----------------------------------------------------------------------------*/
var float_rounding_mode int8t = float_round_nearest_even
var float_exception_flags int8t = 0

/*----------------------------------------------------------------------------
| Constants (from softfloat.h)
*----------------------------------------------------------------------------*/
const (
	float_tininess_after_rounding  = 0
	float_tininess_before_rounding = 1

	float_round_nearest_even = 0
	float_round_to_zero      = 1
	float_round_up           = 2
	float_round_down         = 3

	float_flag_inexact   = 1
	float_flag_divbyzero = 2
	float_flag_underflow = 4
	float_flag_overflow  = 8
	float_flag_invalid   = 16
)

/*----------------------------------------------------------------------------
| Specialized constants (from softfloat-specialize.txt)
*----------------------------------------------------------------------------*/
const float_detect_tininess = float_tininess_before_rounding

var float64_default_nan bits64 = 0x7FFFFFFFFFFFFFFF

/*----------------------------------------------------------------------------
| Helper functions for bool to int conversions (Go requires explicit conversion)
*----------------------------------------------------------------------------*/
func bool2bits64(b bool) bits64 {
	if b {
		return 1
	}
	return 0
}

func bool2flag(b bool) flag {
	if b {
		return 1
	}
	return 0
}

/*----------------------------------------------------------------------------
| Helper macros as functions (LIT64 not needed in Go)
*----------------------------------------------------------------------------*/
func LIT64(a uint64) bits64 {
	return bits64(a)
}

/*----------------------------------------------------------------------------
| Primitive arithmetic functions (from softfloat-macros.txt)
*----------------------------------------------------------------------------*/

func shift64RightJamming(a bits64, count int16t, zPtr *bits64) {
	var z bits64
	if count == 0 {
		z = a
	} else if count < 64 {
		z = (a >> uint(count)) | bool2bits64((a<<(uint((-count)&63))) != 0)
	} else {
		z = bool2bits64(a != 0)
	}
	*zPtr = z
}

func add128(a0, a1, b0, b1 bits64) (z0, z1 bits64) {
	z1 = a1 + b1
	z0 = a0 + b0 + bool2bits64(z1 < a1)
	return
}

func sub128(a0, a1, b0, b1 bits64) (z0, z1 bits64) {
	z1 = a1 - b1
	z0 = a0 - b0 - bool2bits64(a1 < b1)
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
	z0 += (bool2bits64(zMiddleA < zMiddleB) << 32) + (zMiddleA >> 32)
	zMiddleA <<= 32
	z1 += zMiddleA
	z0 += bool2bits64(z1 < zMiddleA)
	return
}

func estimateDiv128To64(a0, a1, b bits64) bits64 {
	b0 := b >> 32
	var z bits64

	if b <= a0 {
		return LIT64(0xFFFFFFFFFFFFFFFF)
	}

	if b0<<32 <= a0 {
		z = LIT64(0xFFFFFFFF00000000)
	} else {
		z = (a0 / b0) << 32
	}

	term0, term1 := mul64To128(b, z)
	rem0, rem1 := sub128(a0, a1, term0, term1)

	for sbits64(rem0) < 0 {
		z -= LIT64(0x100000000)
		b1 := b << 32
		rem0, rem1 = add128(rem0, rem1, b0, b1)
	}

	rem0 = (rem0 << 32) | (rem1 >> 32)
	if b0<<32 <= rem0 {
		z |= 0xFFFFFFFF
	} else {
		z |= rem0 / b0
	}
	return z
}

var countLeadingZerosHigh = [256]int8t{
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

func countLeadingZeros32(a bits32) int8t {
	shiftCount := int8t(0)
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

func countLeadingZeros64(a bits64) int8t {
	shiftCount := int8t(0)
	if a < (bits64(1) << 32) {
		shiftCount += 32
	} else {
		a >>= 32
	}
	shiftCount += countLeadingZeros32(bits32(a))
	return shiftCount
}

/*----------------------------------------------------------------------------
| Specialized functions (from softfloat-specialize.txt)
*----------------------------------------------------------------------------*/

func float_raise(flags int8t) {
	float_exception_flags |= flags
}

func float64_is_nan(a float64t) flag {
	return bool2flag(LIT64(0xFFE0000000000000) < bits64(a<<1))
}

func float64_is_signaling_nan(a float64t) flag {
	return bool2flag((((a>>51)&0xFFF) == 0xFFE) && (bits64(a)&LIT64(0x0007FFFFFFFFFFFF)) != 0)
}

func propagateFloat64NaN(a, b float64t) float64t {
	aIsSignalingNaN := float64_is_signaling_nan(a)
	bIsNaN := float64_is_nan(b)
	bIsSignalingNaN := float64_is_signaling_nan(b)

	a = float64t(bits64(a) | LIT64(0x0008000000000000))
	b = float64t(bits64(b) | LIT64(0x0008000000000000))

	if (aIsSignalingNaN != 0) || (bIsSignalingNaN != 0) {
		float_raise(float_flag_invalid)
	}

	if bIsSignalingNaN != 0 {
		return b
	}
	if aIsSignalingNaN != 0 {
		return a
	}
	if bIsNaN != 0 {
		return b
	}
	return a
}

/*----------------------------------------------------------------------------
| Main softfloat functions (from softfloat.c)
*----------------------------------------------------------------------------*/

func extractFloat64Frac(a float64t) bits64 {
	return bits64(a) & LIT64(0x000FFFFFFFFFFFFF)
}

func extractFloat64Exp(a float64t) int16t {
	return int16t((a >> 52) & 0x7FF)
}

func extractFloat64Sign(a float64t) flag {
	return bool2flag(a>>63 != 0)
}

func normalizeFloat64Subnormal(aSig bits64) (zExp int16t, zSig bits64) {
	shiftCount := countLeadingZeros64(aSig) - 11
	zSig = aSig << uint(shiftCount)
	zExp = int16t(1) - int16t(shiftCount)
	return
}

func packFloat64(zSign flag, zExp int16t, zSig bits64) float64t {
	return float64t((bits64(zSign) << 63) + (bits64(zExp) << 52) + zSig)
}

func roundAndPackFloat64(zSign flag, zExp int16t, zSig bits64) float64t {
	roundingMode := float_rounding_mode
	roundNearestEven := (roundingMode == float_round_nearest_even)
	roundIncrement := int16t(0x200)

	if !roundNearestEven {
		if roundingMode == float_round_to_zero {
			roundIncrement = 0
		} else {
			roundIncrement = 0x3FF
			if zSign != 0 {
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

	roundBits := int16t(zSig & 0x3FF)

	if 0x7FD <= bits16(zExp) {
		if (0x7FD < zExp) || ((zExp == 0x7FD) && (sbits64(zSig+bits64(roundIncrement)) < 0)) {
			float_raise(float_flag_overflow | float_flag_inexact)
			if roundIncrement == 0 {
				return packFloat64(zSign, 0x7FF, 0) - 1
			}
			return packFloat64(zSign, 0x7FF, 0)
		}
		if zExp < 0 {
			isTiny := (float_detect_tininess == float_tininess_before_rounding) ||
				(zExp < -1) ||
				(zSig+bits64(roundIncrement) < LIT64(0x8000000000000000))
			var shiftedSig bits64
			shift64RightJamming(zSig, -zExp, &shiftedSig)
			zSig = shiftedSig
			zExp = 0
			roundBits = int16t(zSig & 0x3FF)
			if isTiny && (roundBits != 0) {
				float_raise(float_flag_underflow)
			}
		}
	}

	if roundBits != 0 {
		float_exception_flags |= float_flag_inexact
	}

	zSig = (zSig + bits64(roundIncrement)) >> 10
	if ((roundBits ^ 0x200) == 0) && roundNearestEven {
		zSig &= ^bits64(1)
	}
	if zSig == 0 {
		zExp = 0
	}
	return packFloat64(zSign, zExp, zSig)
}

func float64_div(a, b float64t) float64t {
	aSig := extractFloat64Frac(a)
	aExp := extractFloat64Exp(a)
	aSign := extractFloat64Sign(a)

	bSig := extractFloat64Frac(b)
	bExp := extractFloat64Exp(b)
	bSign := extractFloat64Sign(b)

	zSign := aSign ^ bSign

	if aExp == 0x7FF {
		if aSig != 0 {
			return propagateFloat64NaN(a, b)
		}
		if bExp == 0x7FF {
			if bSig != 0 {
				return propagateFloat64NaN(a, b)
			}
			float_raise(float_flag_invalid)
			return float64t(float64_default_nan)
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
			if (aExp | int16t(aSig)) == 0 {
				float_raise(float_flag_invalid)
				return float64t(float64_default_nan)
			}
			float_raise(float_flag_divbyzero)
			return packFloat64(zSign, 0x7FF, 0)
		}
		var normExp int16t
		var normSig bits64
		normExp, normSig = normalizeFloat64Subnormal(bSig)
		bExp = normExp
		bSig = normSig
	}

	if aExp == 0 {
		if aSig == 0 {
			return packFloat64(zSign, 0, 0)
		}
		var normExp int16t
		var normSig bits64
		normExp, normSig = normalizeFloat64Subnormal(aSig)
		aExp = normExp
		aSig = normSig
	}

	zExp := aExp - bExp + 0x3FD
	aSig = (aSig | LIT64(0x0010000000000000)) << 10
	bSig = (bSig | LIT64(0x0010000000000000)) << 11

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
		if rem1 != 0 {
			zSig |= 1
		}
	}

	return roundAndPackFloat64(zSign, zExp, zSig)
}

/*----------------------------------------------------------------------------
| Test and Main (from dfdiv.c)
*----------------------------------------------------------------------------*/

func ullongToDouble(x bits64) float64 {
	return math.Float64frombits(uint64(x))
}

const N = 22

var a_input = [N]float64t{
	0x7FFF000000000000, /* nan */
	0x7FF0000000000000, /* inf */
	0x7FF0000000000000, /* inf */
	0x7FF0000000000000, /* inf */
	0x3FF0000000000000, /* 1.0 */
	0x3FF0000000000000, /* 1.0 */
	0x0000000000000000, /* 0.0 */
	0x3FF0000000000000, /* 1.0 */
	0x0000000000000000, /* 0.0 */
	0x8000000000000000, /* -0.0 */
	0x4008000000000000, /* 3.0 */
	0xC008000000000000, /* -3.0 */
	0x4008000000000000, /* 3.0 */
	0xC008000000000000, /* -3.0 */
	0x4000000000000000, /* 2.0 */
	0xC000000000000000, /* -2.0 */
	0x4000000000000000, /* 2.0 */
	0xC000000000000000, /* -2.0 */
	0x3FF0000000000000, /* 1.0 */
	0xBFF0000000000000, /* -1.0 */
	0x3FF0000000000000, /* 1.0 */
	0xBFF0000000000000, /* -1.0 */
}

var b_input = [N]float64t{
	0x3FF0000000000000, /* 1.0 */
	0x7FF8000000000000, /* nan */
	0x7FF0000000000000, /* inf */
	0x3FF0000000000000, /* 1.0 */
	0x7FF8000000000000, /* nan */
	0x7FF0000000000000, /* inf */
	0x0000000000000000, /* 0.0 */
	0x0000000000000000, /* 0.0 */
	0x3FF0000000000000, /* 1.0 */
	0x3FF0000000000000, /* 1.0 */
	0x4000000000000000, /* 2.0 */
	0x4000000000000000, /* 2.0 */
	0xC000000000000000, /* 2.0 */
	0xC000000000000000, /* -2.0 */
	0x4010000000000000, /* 4.0 */
	0x4010000000000000, /* 4.0 */
	0xC010000000000000, /* -4.0 */
	0xC010000000000000, /* -4.0 */
	0x3FF8000000000000, /* 1.5 */
	0x3FF8000000000000, /* 1.5 */
	0xBFF8000000000000, /* -1.5 */
	0xBFF8000000000000, /* -1.5 */
}

var z_output = [N]float64t{
	0x7FFF000000000000, /* nan */
	0x7FF8000000000000, /* nan */
	0x7FFFFFFFFFFFFFFF, /* nan */
	0x7FF0000000000000, /* inf */
	0x7FF8000000000000, /* nan */
	0x0000000000000000, /* 0.0 */
	0x7FFFFFFFFFFFFFFF, /* nan */
	0x7FF0000000000000, /* inf */
	0x0000000000000000, /* 0.0 */
	0x8000000000000000, /* -0.0 */
	0x3FF8000000000000, /* 1.5 */
	0xBFF8000000000000, /* -1.5 */
	0xBFF8000000000000, /* 1.5 */
	0x3FF8000000000000, /* -1.5 */
	0x3FE0000000000000, /* 0.5 */
	0xBFE0000000000000, /* 5.0 */
	0xBFE0000000000000, /* -5.0 */
	0x3FE0000000000000, /* 0.5 */
	0x3FE5555555555555, /* 0.666667 */
	0xBFE5555555555555, /* -0.666667 */
	0xBFE5555555555555, /* -0.666667 */
	0x3FE5555555555555, /* 0.666667 */
}

func main() {
	main_result := 0
	var x1, x2 float64t

	for i := 0; i < N; i++ {
		x1 = a_input[i]
		x2 = b_input[i]
		result := float64_div(x1, x2)
		if result != z_output[i] {
			main_result++
		}

		fmt.Printf("a_input=%016x b_input=%016x expected=%016x output=%016x (%f)\n",
			uint64(a_input[i]), uint64(b_input[i]), uint64(z_output[i]), uint64(result),
			ullongToDouble(bits64(result)))
	}
	fmt.Printf("%d\n", main_result)
}
