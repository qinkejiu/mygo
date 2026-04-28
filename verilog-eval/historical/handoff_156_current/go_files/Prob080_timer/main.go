package main

var out_tc bool

func TopModule(clk bool, load bool, data uint16) {
    // Internal counter register (10 bits)
    var count uint16

    // Sequential logic triggered on positive edge of clk
    if clk {
        if load {
            // Load the 10-bit data into counter
            count = data & 0x3FF // Mask to ensure only 10 bits
        } else {
            // Decrement if not zero
            if count != 0 {
                count = count - 1
            }
        }
    }

    // Combinational output: tc is true when counter reaches 0
    out_tc = (count == 0)
}

func main() {}
