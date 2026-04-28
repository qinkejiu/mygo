package main

var out_q uint8

func TopModule(clk bool, shift_ena bool, count_ena bool, data bool) {
    // Internal register state
    var reg uint8 = 0

    // On positive edge of clock
    if clk {
        if shift_ena {
            // Shift left and insert data at LSB (MSB-first shifting)
            // q[2:0] becomes new q[3:1], data becomes new q[0]
            reg = ((reg << 1) & 0x0E) | uint8(boolToBit(data))
        } else if count_ena {
            // Decrement the 4-bit value
            reg = (reg - 1) & 0x0F
        }
        // If neither shift_ena nor count_ena is true, reg retains its value
    }

    out_q = reg & 0x0F
}

// Helper function to convert bool to 0/1 bit
func boolToBit(b bool) uint8 {
    if b {
        return 1
    }
    return 0
}

func main() {}
