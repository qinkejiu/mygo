package main

var out_q uint64

func TopModule(clk bool, load bool, ena bool, amount uint8, data uint64) {
    // Internal register state
    var reg uint64 = 0

    // Clocked behavior
    if clk {
        if load {
            reg = data
        } else if ena {
            switch amount {
            case 0x00: // shift left by 1 bit
                reg = (reg << 1) & 0xFFFFFFFFFFFFFFFF
            case 0x01: // shift left by 8 bits
                reg = (reg << 8) & 0xFFFFFFFFFFFFFFFF
            case 0x02: // shift right by 1 bit (arithmetic)
                // For arithmetic right shift, we need to preserve sign bit
                signBit := reg & 0x8000000000000000
                reg = (reg >> 1) | signBit
            case 0x03: // shift right by 8 bits (arithmetic)
                // For arithmetic right shift by 8, preserve sign bit and extend
                signBit := reg & 0x8000000000000000
                if signBit != 0 {
                    reg = (reg >> 8) | 0xFF00000000000000
                } else {
                    reg = reg >> 8
                }
            }
        }
        out_q = reg
    }
}

func main() {}
