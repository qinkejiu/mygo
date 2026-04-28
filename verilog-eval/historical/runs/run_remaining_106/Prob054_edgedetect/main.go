package main

var out_pedge uint8

func TopModule(clk bool, in uint8) {
    var d_last uint8 = 0
    
    // Sequential logic using ordinary Go conditionals
    if clk {
        // Positive edge detection: in & ~d_last
        out_pedge = in & ^d_last
        d_last = in
    }
}

func main() {}
