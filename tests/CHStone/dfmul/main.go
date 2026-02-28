package main

import (
	"fmt"
	"math"
	"math/bits"
)

/*----------------------------------------------------------------------------
| 类型定义 (对应 SPARC-GCC.h 和 softfloat.h)
*----------------------------------------------------------------------------*/
type flag = uint8
type int8_t = int8   // 改名为 int8_t 避免与内置 int8 冲突
type int16_t = int16 // 改名为 int16_t 避免与内置 int16 冲突
type bits16 = uint16
type bits32 = uint32
type bits64 = uint64
type sbits64 = int64
type float32_t = uint32 // 改名为 float32_t
type float64_t = uint64 // 改名为 float64_t，避免与 Go 的 float64 冲突

/*----------------------------------------------------------------------------
| 常数定义
*----------------------------------------------------------------------------*/

// 舍入模式
const (
	floatRoundNearestEven = 0
	floatRoundToZero      = 1
	floatRoundUp          = 2
	floatRoundDown        = 3
)

// 下溢检测模式
const (
	floatTininessAfterRounding  = 0
	floatTininessBeforeRounding = 1
)

// 异常标志
const (
	floatFlagInexact   = 1
	floatFlagDivbyzero = 2
	floatFlagUnderflow = 4
	floatFlagOverflow  = 8
	floatFlagInvalid   = 16
)

// 全局状态
var floatRoundingMode = floatRoundNearestEven
var floatExceptionFlags int8_t = 0
var floatDetectTininess = floatTininessBeforeRounding

// 默认NaN模式
const float64DefaultNan = uint64(0x7FFFFFFFFFFFFFFF)

/*----------------------------------------------------------------------------
| 基础工具函数 (对应 softfloat-macros.txt)
*----------------------------------------------------------------------------*/

// shift64RightJamming 将a右移count位，移出的位如果非零则sticky到最低位
func shift64RightJamming(a bits64, count int16_t) bits64 {
	if count == 0 {
		return a
	} else if count < 64 {
		return (a >> count) | boolToUint64((a<<((-count)&63)) != 0)
	} else {
		return boolToUint64(a != 0)
	}
}

// mul64To128 64位乘64位得到128位结果，分为高64位和低64位
func mul64To128(a, b bits64) (z0, z1 bits64) {
	// Go的math/bits.Mul64直接提供此功能
	hi, lo := bits.Mul64(a, b)
	return hi, lo
}

// countLeadingZeros32 返回32位整数前导零个数
func countLeadingZeros32(a bits32) int8_t {
	if a == 0 {
		return 32
	}
	return int8_t(bits.Len32(a)) - 1
}

// countLeadingZeros64 返回64位整数前导零个数
func countLeadingZeros64(a bits64) int8_t {
	if a == 0 {
		return 64
	}
	return int8_t(63 - bits.Len64(a))
}

/*----------------------------------------------------------------------------
| 特殊化处理 (对应 softfloat-specialize.txt)
*----------------------------------------------------------------------------*/

// floatRaise 设置异常标志
func floatRaise(flags int8_t) {
	floatExceptionFlags |= flags
}

// float64IsNan 检查是否为NaN
func float64IsNan(a float64_t) flag {
	return boolToUint8((a << 1) > 0xFFE0000000000000)
}

// float64IsSignalingNan 检查是否为信号NaN
func float64IsSignalingNan(a float64_t) flag {
	return boolToUint8(((a>>51)&0xFFF) == 0xFFE && (a&0x0007FFFFFFFFFFFF) != 0)
}

// propagateFloat64Nan 处理NaN传播 (注意大小写：Nan 不是 NaN)
func propagateFloat64Nan(a, b float64_t) float64_t {
	aIsNan := float64IsNan(a)
	aIsSignalingNan := float64IsSignalingNan(a)
	bIsNan := float64IsNan(b)
	bIsSignalingNan := float64IsSignalingNan(b)

	// 使用 aIsNan 避免未使用错误
	_ = aIsNan

	a |= 0x0008000000000000
	b |= 0x0008000000000000

	if aIsSignalingNan != 0 || bIsSignalingNan != 0 {
		floatRaise(floatFlagInvalid)
	}

	if bIsSignalingNan != 0 {
		return b
	} else if aIsSignalingNan != 0 {
		return a
	} else if bIsNan != 0 {
		return b
	} else {
		return a
	}
}

/*----------------------------------------------------------------------------
| 浮点操作辅助函数 (对应 softfloat.c)
*----------------------------------------------------------------------------*/

// extractFloat64Frac 提取尾数（小数部分）
func extractFloat64Frac(a float64_t) bits64 {
	return a & 0x000FFFFFFFFFFFFF
}

// extractFloat64Exp 提取指数部分
func extractFloat64Exp(a float64_t) int16_t {
	return int16_t((a >> 52) & 0x7FF)
}

// extractFloat64Sign 提取符号位
func extractFloat64Sign(a float64_t) flag {
	return flag(a >> 63)
}

// normalizeFloat64Subnormal 规格化次正规数
func normalizeFloat64Subnormal(aSig bits64) (int16_t, bits64) {
	shiftCount := countLeadingZeros64(aSig) - 11
	return 1 - int16_t(shiftCount), aSig << shiftCount
}

// packFloat64 打包浮点数
func packFloat64(zSign flag, zExp int16_t, zSig bits64) float64_t {
	return (uint64(zSign) << 63) + (uint64(zExp) << 52) + zSig
}

// roundAndPackFloat64 舍入并打包
func roundAndPackFloat64(zSign flag, zExp int16_t, zSig bits64) float64_t {
	roundingMode := floatRoundingMode
	roundNearestEven := (roundingMode == floatRoundNearestEven)
	var roundIncrement int16_t = 0x200

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

	roundBits := int16_t(zSig & 0x3FF)

	if uint16(zExp) >= 0x7FD {
		if (0x7FD < zExp) || (zExp == 0x7FD && int64(zSig+uint64(roundIncrement)) < 0) {
			floatRaise(floatFlagOverflow | floatFlagInexact)
			if roundIncrement == 0 {
				return packFloat64(zSign, 0x7FF, 0)
			}
			return packFloat64(zSign, 0x7FF, 0) - 1
		}
		if zExp < 0 {
			isTiny := (floatDetectTininess == floatTininessBeforeRounding) ||
				(zExp < -1) ||
				(int64(zSig+uint64(roundIncrement)) < 0 && zSig+uint64(roundIncrement) < 0x8000000000000000)

			zSig = shift64RightJamming(zSig, -zExp)
			zExp = 0
			roundBits = int16_t(zSig & 0x3FF)
			if isTiny && roundBits != 0 {
				floatRaise(floatFlagUnderflow)
			}
		}
	}

	if roundBits != 0 {
		floatExceptionFlags |= floatFlagInexact
	}

	zSig = (zSig + uint64(roundIncrement)) >> 10
	
	// 修复：使用条件判断代替布尔值移位
	if ((roundBits^0x200) == 0) && roundNearestEven {
		zSig &^= 1 // 清除最低位（实现偶数舍入）
	}

	if zSig == 0 {
		zExp = 0
	}

	return packFloat64(zSign, zExp, zSig)
}


// float64Mul 双精度浮点乘法
func float64Mul(a, b float64_t) float64_t {
	aSig := extractFloat64Frac(a)
	aExp := extractFloat64Exp(a)
	aSign := extractFloat64Sign(a)

	bSig := extractFloat64Frac(b)
	bExp := extractFloat64Exp(b)
	bSign := extractFloat64Sign(b)

	zSign := aSign ^ bSign

	// 处理无穷大
	if aExp == 0x7FF {
		if aSig != 0 || (bExp == 0x7FF && bSig != 0) {
			return propagateFloat64Nan(a, b)
		}
		if (bExp | int16_t(boolToUint16(bSig != 0))) == 0 {
			floatRaise(floatFlagInvalid)
			return float64DefaultNan
		}
		return packFloat64(zSign, 0x7FF, 0)
	}

	if bExp == 0x7FF {
		if bSig != 0 {
			return propagateFloat64Nan(a, b)
		}
		if (aExp | int16_t(boolToUint16(aSig != 0))) == 0 {
			floatRaise(floatFlagInvalid)
			return float64DefaultNan
		}
		return packFloat64(zSign, 0x7FF, 0)
	}

	// 处理零和次正规数
	if aExp == 0 {
		if aSig == 0 {
			return packFloat64(zSign, 0, 0)
		}
		var newExp int16_t
		newExp, aSig = normalizeFloat64Subnormal(aSig)
		aExp = newExp
	}

	if bExp == 0 {
		if bSig == 0 {
			return packFloat64(zSign, 0, 0)
		}
		var newExp int16_t
		newExp, bSig = normalizeFloat64Subnormal(bSig)
		bExp = newExp
	}

	zExp := aExp + bExp - 0x3FF
	aSig = (aSig | 0x0010000000000000) << 10
	bSig = (bSig | 0x0010000000000000) << 11

	zSig0, zSig1 := mul64To128(aSig, bSig)
	if zSig1 != 0 {
		zSig0 |= 1
	}

	if int64(zSig0<<1) >= 0 {
		zSig0 <<= 1
		zExp--
	}

	return roundAndPackFloat64(zSign, zExp, zSig0)
}

/*----------------------------------------------------------------------------
| 辅助函数
*----------------------------------------------------------------------------*/

func boolToUint8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

func boolToUint16(b bool) uint16 {
	if b {
		return 1
	}
	return 0
}

func boolToUint64(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

// ullongToDouble 将uint64位模式转换为Go的float64
func ullongToDouble(x uint64) float64 {
	return math.Float64frombits(x)
}

/*----------------------------------------------------------------------------
| 测试代码 (对应 dfmul.c)
*----------------------------------------------------------------------------*/

const N = 20

var aInput = [N]uint64{
	0x7FF0000000000000, // inf
	0x7FFF000000000000, // nan
	0x7FF0000000000000, // inf
	0x7FF0000000000000, // inf
	0x3FF0000000000000, // 1.0
	0x0000000000000000, // 0.0
	0x3FF0000000000000, // 1.0
	0x0000000000000000, // 0.0
	0x8000000000000000, // -0.0
	0x3FF0000000000000, // 1.0
	0x3FF0000000000000, // 1.0
	0x4000000000000000, // 2.0
	0x3FD0000000000000, // 0.25
	0xC000000000000000, // -2.0
	0xBFD0000000000000, // -0.25
	0x4000000000000000, // 2.0
	0xBFD0000000000000, // -0.25
	0xC000000000000000, // -2.0
	0x3FD0000000000000, // 0.25
	0x0000000000000000, // 0.0
}

var bInput = [N]uint64{
	0xFFFFFFFFFFFFFFFF, // nan
	0xFFF0000000000000, // -inf
	0x0000000000000000, // nan (实际上应该是0，但原代码标记为nan)
	0x3FF0000000000000, // 1.0
	0xFFFF000000000000, // nan
	0x7FF0000000000000, // inf
	0x7FF0000000000000, // inf
	0x3FF0000000000000, // 1.0
	0x3FF0000000000000, // 1.0
	0x0000000000000000, // 0.0
	0x8000000000000000, // -0.0
	0x3FD0000000000000, // 0.25
	0x4000000000000000, // 2.0
	0xBFD0000000000000, // -0.25
	0xC000000000000000, // -2.0
	0xBFD0000000000000, // -0.25
	0x4000000000000000, // 2.0
	0x3FD0000000000000, // 0.25
	0xC000000000000000, // -2.0
	0x0000000000000000, // 0.0
}

var zOutput = [N]uint64{
	0xFFFFFFFFFFFFFFFF, // nan
	0x7FFF000000000000, // nan
	0x7FFFFFFFFFFFFFFF, // nan
	0x7FF0000000000000, // inf
	0xFFFF000000000000, // nan
	0x7FFFFFFFFFFFFFFF, // nan
	0x7FF0000000000000, // inf
	0x0000000000000000, // 0.0
	0x8000000000000000, // -0.0
	0x0000000000000000, // 0.0
	0x8000000000000000, // -0.0
	0x3FE0000000000000, // 0.5
	0x3FE0000000000000, // 0.5
	0x3FE0000000000000, // 0.5
	0x3FE0000000000000, // 0.5
	0xBFE0000000000000, // -0.5
	0xBFE0000000000000, // -0.5
	0xBFE0000000000000, // -0.5
	0xBFE0000000000000, // -0.5
	0x0000000000000000, // 0.0
}

func main() {
	mainResult := 0

	for i := 0; i < N; i++ {
		x1 := aInput[i]
		x2 := bInput[i]
		result := float64Mul(x1, x2)

		if result != zOutput[i] {
			mainResult++
		}

		fmt.Printf("a_input=%016x b_input=%016x expected=%016x output=%016x (%f)\n",
			aInput[i], bInput[i], zOutput[i], result, ullongToDouble(result))
	}

	fmt.Printf("Errors: %d\n", mainResult)
}
