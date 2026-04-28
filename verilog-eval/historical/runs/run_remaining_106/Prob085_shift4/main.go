package main

var out_q uint8

func TopModule(clk bool, areset bool, load bool, ena bool, data uint8) {
    // Internal state for the shift register
    var reg uint8 = 0

    // Asynchronous reset (positive edge triggered)
    if areset {
        reg = 0
    } else if clk {
        // Synchronous logic on positive clock edge
        if load {
            reg = data & 0x0F // Only keep lower 4 bits
        } else if ena {
            // Right shift: q[3] becomes 0, q[0] is shifted out
            reg = (reg >> 1) & 0x07 // Shift right and clear new MSB
        }
    }

    // Assign to output global
    out_q = reg & 0x0F // Ensure only 4 bits are output
}

func main() {}
