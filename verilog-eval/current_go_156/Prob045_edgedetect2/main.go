package main

var out_anyedge uint8

func TopModule(clk bool, in uint8) {
    var d_last uint8 = 0
    
    // Sequential logic triggered on positive edge of clk
    if clk {
        // Calculate edge detection: XOR between current input and previous value
        out_anyedge = in ^ d_last
        
        // Store current input for next cycle
        d_last = in
    }
}

func main() {}
