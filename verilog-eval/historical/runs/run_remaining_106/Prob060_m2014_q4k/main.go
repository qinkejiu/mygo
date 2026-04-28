package main

var out_out bool

func TopModule(clk bool, resetn bool, in bool) {
    // Four D flip-flops represented as individual bits
    var d0, d1, d2, d3 bool
    
    // Sequential logic triggered on positive edge of clk
    // In Go, we simulate this by checking if clk is true (positive edge)
    // and updating the flip-flops accordingly
    if clk {
        if !resetn {
            // Active-low synchronous reset
            d0 = false
            d1 = false
            d2 = false
            d3 = false
        } else {
            // Shift register behavior
            d3 = d2
            d2 = d1
            d1 = d0
            d0 = in
        }
    }
    
    // Output is the most significant bit (sr[3] in Verilog)
    out_out = d3
}

func main() {}
