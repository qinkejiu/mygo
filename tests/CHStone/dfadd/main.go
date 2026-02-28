package main

import (
	"fmt"
	"math"
)

/*----------------------------------------------------------------------------
| 类型定义
*----------------------------------------------------------------------------*/
type flag uint8
type bits16 uint16
type bits32 uint32
type bits64 uint64
type sbits64 int64

type Float32 uint32
type Float64 uint64

/*----------------------------------------------------------------------------
| 全局状态变量
*----------------------------------------------------------------------------*/
var (
	floatRoundingMode   int8 = 0
	floatExceptionFlags int8 = 0
)

/*----------------------------------------------------------------------------
| 常量定义
*----------------------------------------------------------------------------*/
const (
	floatRoundNearestEven int8 = 0
	floatRoundToZero      int8 = 1
	floatRoundUp          int8 = 2
	floatRoundDown        int8 = 3

	floatFlagInexact   int8 = 1
	floatFlagDivbyzero int8 = 2
	floatFlagUnderflow int8 = 4
	floatFlagOverflow  int8 = 8
	floatFlagInvalid   int8 = 16

	floatTininessBeforeRounding int8 = 1
	floatDetectTininess         int8 = floatTininessBeforeRounding

	float64DefaultNan bits64 = 0x7FFFFFFFFFFFFFFF
)

/*----------------------------------------------------------------------------
| 位操作原语
*----------------------------------------------------------------------------*/
func shift64RightJamming(a bits64, count int16) bits64 {
	if count == 0 {
		return a
	} else if count < 64 {
		hasBits := (a << (64 - uint16(count))) != 0
		z := a >> uint16(count)
		if hasBits {
			z |= 1
		}
		return z
	} else {
		if a != 0 {
			return 1
		}
		return 0
	}
}

var countLeadingZerosHigh = [256]uint8{
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

func countLeadingZeros32(a bits32) int8 {
	shiftCount := int8(0)
	if a < 0x10000 {
		shiftCount += 16
		a <<= 16
	}
	if a < 0x1000000 {
		shiftCount += 8
		a <<= 8
	}
	shiftCount += int8(countLeadingZerosHigh[a>>24])
	return shiftCount
}

func countLeadingZeros64(a bits64) int8 {
	shiftCount := int8(0)
	if a < (bits64(1) << 32) {
		shiftCount += 32
	} else {
		a >>= 32
	}
	shiftCount += countLeadingZeros32(bits32(a))
	return shiftCount
}

/*----------------------------------------------------------------------------
| 异常处理和专门化
*----------------------------------------------------------------------------*/
func floatRaise(flags int8) {
	floatExceptionFlags |= flags
}

func float64IsNan(a Float64) flag {
	if 0xFFE0000000000000 < bits64(a<<1) {
		return 1
	}
	return 0
}

func float64IsSignalingNan(a Float64) flag {
	if (((a >> 51) & 0xFFF) == 0xFFE) && (a&0x0007FFFFFFFFFFFF) != 0 {
		return 1
	}
	return 0
}

func propagateFloat64NaN(a, b Float64) Float64 {
	aIsSignalingNaN := float64IsSignalingNan(a)
	bIsNaN := float64IsNan(b)
	bIsSignalingNaN := float64IsSignalingNan(b)

	a |= 0x0008000000000000
	b |= 0x0008000000000000

	if (aIsSignalingNaN | bIsSignalingNaN) != 0 {
		floatRaise(floatFlagInvalid)
	}

	if bIsSignalingNaN != 0 {
		return b
	} else if aIsSignalingNaN != 0 {
		return a
	} else if bIsNaN != 0 {
		return b
	}
	return a
}

/*----------------------------------------------------------------------------
| 浮点操作辅助函数
*----------------------------------------------------------------------------*/
func extractFloat64Frac(a Float64) bits64 {
	return bits64(a & 0x000FFFFFFFFFFFFF)
}

func extractFloat64Exp(a Float64) int16 {
	return int16((a >> 52) & 0x7FF)
}

func extractFloat64Sign(a Float64) flag {
	return flag(a >> 63)
}

func packFloat64(zSign flag, zExp int16, zSig bits64) Float64 {
	return Float64((bits64(zSign) << 63) + (bits64(zExp) << 52) + zSig)
}

func roundAndPackFloat64(zSign flag, zExp int16, zSig bits64) Float64 {
	roundingMode := floatRoundingMode
	roundNearestEven := (roundingMode == floatRoundNearestEven)
	var roundIncrement int16 = 0x200
	if !roundNearestEven {
		if roundingMode == floatRoundToZero {
			roundIncrement = 0
		} else {
			roundIncrement = 0x3FF
			if zSign != 0 {
				if roundingMode == floatRoundUp {
					roundIncrement = 0
				}
			} else {
				if roundingMode == floatRoundDown {
					roundIncrement = 0
				}
			}
		}
	}
	roundBits := int16(zSig & 0x3FF)

	if 0x7FD <= uint16(zExp) {
		if (0x7FD < zExp) || ((zExp == 0x7FD) && (sbits64(zSig+bits64(uint64(roundIncrement))) < 0)) {
			floatRaise(floatFlagOverflow | floatFlagInexact)
			if roundIncrement == 0 {
				return packFloat64(zSign, 0x7FF, 0)
			}
			return packFloat64(zSign, 0x7FF, 0x000FFFFFFFFFFFFF)
		}
		if zExp < 0 {
			isTiny := (floatDetectTininess == floatTininessBeforeRounding) ||
				(zExp < -1) ||
				((zSig + bits64(uint64(roundIncrement))) < 0x8000000000000000)
			zSig = shift64RightJamming(zSig, -zExp)
			zExp = 0
			roundBits = int16(zSig & 0x3FF)
			if isTiny && (roundBits != 0) {
				floatRaise(floatFlagUnderflow)
			}
		}
	}

	if roundBits != 0 {
		floatExceptionFlags |= floatFlagInexact
	}
	zSig = (zSig + bits64(uint64(roundIncrement))) >> 10
	if (roundBits ^ 0x200) == 0 && roundNearestEven {
		zSig &= ^bits64(1)
	}
	if zSig == 0 {
		zExp = 0
	}
	return packFloat64(zSign, zExp, zSig)
}

func normalizeRoundAndPackFloat64(zSign flag, zExp int16, zSig bits64) Float64 {
	shiftCount := countLeadingZeros64(zSig) - 1
	return roundAndPackFloat64(zSign, zExp-int16(shiftCount), zSig<<uint8(shiftCount))
}

/*----------------------------------------------------------------------------
| 浮点加减法核心实现
*----------------------------------------------------------------------------*/
func addFloat64Sigs(a, b Float64, zSign flag) Float64 {
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
		zExp := aExp
		aSig |= 0x2000000000000000
		zSig := (aSig + bSig) << 1
		zExp--
		if sbits64(zSig) < 0 {
			zSig = aSig + bSig
			zExp++
		}
		return roundAndPackFloat64(zSign, zExp, zSig)
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
		zExp := bExp
		bSig |= 0x2000000000000000
		zSig := (aSig + bSig) << 1
		zExp--
		if sbits64(zSig) < 0 {
			zSig = aSig + bSig
			zExp++
		}
		return roundAndPackFloat64(zSign, zExp, zSig)
	} else {
		if aExp == 0x7FF {
			if (aSig | bSig) != 0 {
				return propagateFloat64NaN(a, b)
			}
			return a
		}
		if aExp == 0 {
			return packFloat64(zSign, 0, (aSig+bSig)>>9)
		}
		zSig := bits64(0x4000000000000000) + aSig + bSig
		zExp := aExp
		// 关键修复：删除错误的 <<1，指数相同时不需要左移
		return roundAndPackFloat64(zSign, zExp, zSig)
	}
}

func subFloat64Sigs(a, b Float64, zSign flag) Float64 {
	aSig := extractFloat64Frac(a)
	aExp := extractFloat64Exp(a)
	bSig := extractFloat64Frac(b)
	bExp := extractFloat64Exp(b)

	expDiff := aExp - bExp
	aSig <<= 10
	bSig <<= 10

	var zSig bits64
	var zExp int16

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
			bSig |= 0x4000000000000000
		}
		bSig = shift64RightJamming(bSig, expDiff)
		aSig |= 0x4000000000000000
		zSig = aSig - bSig
		zExp = aExp
		zExp--
		return normalizeRoundAndPackFloat64(zSign, zExp, zSig)
	} else if expDiff < 0 {
		if bExp == 0x7FF {
			if bSig != 0 {
				return propagateFloat64NaN(a, b)
			}
			return packFloat64(zSign^1, 0x7FF, 0)
		}
		if aExp == 0 {
			expDiff++
		} else {
			aSig |= 0x4000000000000000
		}
		aSig = shift64RightJamming(aSig, -expDiff)
		bSig |= 0x4000000000000000
		zSig = bSig - aSig
		zExp = bExp
		zSign ^= 1
		zExp--
		return normalizeRoundAndPackFloat64(zSign, zExp, zSig)
	} else {
		if aExp == 0x7FF {
			if (aSig | bSig) != 0 {
				return propagateFloat64NaN(a, b)
			}
			floatRaise(floatFlagInvalid)
			return Float64(float64DefaultNan)
		}
		if aExp == 0 {
			aExp = 1
			bExp = 1
		}
		if bSig < aSig {
			zSig = aSig - bSig
			zExp = aExp
			zExp--
			return normalizeRoundAndPackFloat64(zSign, zExp, zSig)
		} else if aSig < bSig {
			zSig = bSig - aSig
			zExp = bExp
			zSign ^= 1
			zExp--
			return normalizeRoundAndPackFloat64(zSign, zExp, zSig)
		} else {
			if floatRoundingMode == floatRoundDown {
				return packFloat64(1, 0, 0)
			}
			return packFloat64(0, 0, 0)
		}
	}
}

func Float64Add(a, b Float64) Float64 {
	aSign := extractFloat64Sign(a)
	bSign := extractFloat64Sign(b)

	if aSign == bSign {
		return addFloat64Sigs(a, b, aSign)
	} else {
		return subFloat64Sigs(a, b, aSign)
	}
}

/*----------------------------------------------------------------------------
| 辅助函数
*----------------------------------------------------------------------------*/
func ullongToDouble(x uint64) float64 {
	return math.Float64frombits(x)
}

/*----------------------------------------------------------------------------
| 测试用例
*----------------------------------------------------------------------------*/
const N = 46

var aInput = [N]Float64{
	0x7FF8000000000000, 0x7FF0000000000000, 0x4000000000000000, 0x4000000000000000,
	0x3FF0000000000000, 0x3FF0000000000000, 0x0000000000000000, 0x3FF8000000000000,
	0x7FF8000000000000, 0x7FF0000000000000, 0x0000000000000000, 0x3FF8000000000000,
	0xFFF8000000000000, 0xFFF0000000000000, 0xC000000000000000, 0xC000000000000000,
	0xBFF0000000000000, 0xBFF0000000000000, 0x8000000000000000, 0xBFF8000000000000,
	0xFFF8000000000000, 0xFFF0000000000000, 0x8000000000000000, 0xBFF8000000000000,
	0x7FF8000000000000, 0x7FF0000000000000, 0x3FF0000000000000, 0x3FF0000000000000,
	0x3FF0000000000000, 0x0000000000000000, 0x3FF8000000000000, 0x7FF8000000000000,
	0x7FF0000000000000, 0x3FF0000000000000, 0x4000000000000000, 0xFFF0000000000000,
	0xFFF0000000000000, 0xBFF0000000000000, 0xBFF0000000000000, 0xBFF0000000000000,
	0x8000000000000000, 0xBFF8000000000000, 0xFFF8000000000000, 0xFFF0000000000000,
	0xBFF0000000000000, 0xC000000000000000,
}

var bInput = [N]Float64{
	0x3FF0000000000000, 0x3FF0000000000000, 0x0000000000000000, 0x3FF8000000000000,
	0x7FF8000000000000, 0x7FF0000000000000, 0x4000000000000000, 0x4000000000000000,
	0x7FF0000000000000, 0x7FF0000000000000, 0x0000000000000000, 0x3FF0000000000000,
	0xBFF0000000000000, 0xBFF0000000000000, 0x8000000000000000, 0xBFF8000000000000,
	0xFFF8000000000000, 0xFFF0000000000000, 0xC000000000000000, 0xC000000000000000,
	0xFFF0000000000000, 0xFFF0000000000000, 0x8000000000000000, 0xBFF0000000000000,
	0xFFF0000000000000, 0xFFF0000000000000, 0xBFF0000000000000, 0xFFF8000000000000,
	0xFFF0000000000000, 0xBFF0000000000000, 0xC000000000000000, 0xBFF0000000000000,
	0xBFF0000000000000, 0x8000000000000000, 0xBFF8000000000000, 0x7FF8000000000000,
	0x7FF0000000000000, 0x3FF0000000000000, 0x7FF8000000000000, 0x7FF0000000000000,
	0x3FF0000000000000, 0x4000000000000000, 0x3FF0000000000000, 0x3FF0000000000000,
	0x0000000000000000, 0x3FF8000000000000,
}

var zOutput = [N]Float64{
	0x7FF8000000000000, 0x7FF0000000000000, 0x4000000000000000, 0x400C000000000000,
	0x7FF8000000000000, 0x7FF0000000000000, 0x4000000000000000, 0x400C000000000000,
	0x7FF8000000000000, 0x7FF0000000000000, 0x0000000000000000, 0x4004000000000000,
	0xFFF8000000000000, 0xFFF0000000000000, 0xC000000000000000, 0xC00C000000000000,
	0xFFF8000000000000, 0xFFF0000000000000, 0xC000000000000000, 0xC00C000000000000,
	0xFFF8000000000000, 0xFFF0000000000000, 0x8000000000000000, 0xC004000000000000,
	0x7FF8000000000000, 0x7FFFFFFFFFFFFFFF, 0x0000000000000000, 0xFFF8000000000000,
	0xFFF0000000000000, 0xBFF0000000000000, 0xBFE0000000000000, 0x7FF8000000000000,
	0x7FF0000000000000, 0x3FF0000000000000, 0x3FE0000000000000, 0x7FF8000000000000,
	0x7FFFFFFFFFFFFFFF, 0x0000000000000000, 0x7FF8000000000000, 0x7FF0000000000000,
	0x3FF0000000000000, 0x3FE0000000000000, 0xFFF8000000000000, 0xFFF0000000000000,
	0xBFF0000000000000, 0xBFE0000000000000,
}

func main() {
	mainResult := 0
	for i := 0; i < N; i++ {
		x1 := aInput[i]
		x2 := bInput[i]
		result := Float64Add(x1, x2)
		if result != zOutput[i] {
			mainResult++
		}

		fmt.Printf("a_input=%016x b_input=%016x expected=%016x output=%016x (%f)\n",
			uint64(x1), uint64(x2), uint64(zOutput[i]), uint64(result),
			ullongToDouble(uint64(result)))
	}
	fmt.Printf("\nErrors: %d\n", mainResult)
	if mainResult == 0 {
		fmt.Println("All tests passed!")
	}
}
