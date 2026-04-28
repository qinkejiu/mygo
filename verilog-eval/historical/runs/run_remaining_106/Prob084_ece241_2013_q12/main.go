package main

var out_Z bool

func TopModule(clk bool, enable bool, S bool, A bool, B bool, C bool) {
    // 8-bit shift register state
    var q uint8 = 0

    // Sequential logic for shift register
    if clk {
        if enable {
            // Shift left by 1, insert S at LSB (Q[0])
            // In the reference, S feeds Q[0] and MSB is shifted in first,
            // which means q[7] is the most recent bit.
            // So we shift right and put S at the MSB (bit 7).
            q = (q >> 1) | (uint8(boolToUint8(S)) << 7)
        }
    }

    // Multiplexer: select bit based on A,B,C
    // {A,B,C} forms a 3-bit index where A is MSB, C is LSB
    idx := (boolToUint8(A) << 2) | (boolToUint8(B) << 1) | boolToUint8(C)
    
    // Extract the selected bit
    out_Z = ((q >> (7 - idx)) & 0x1) != 0
}

func boolToUint8(b bool) uint8 {
    if b {
        return 1
    }
    return 0
}

func main() {}
