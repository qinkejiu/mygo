package main

var out_q bool

func TopModule(clk bool, d bool) {
    var qp, qn bool
    // Positive edge triggered flip-flop
    if clk {
        qp = d
    }
    // Negative edge triggered flip-flop  
    if !clk {
        qn = d
    }
    // Mux to select between the two flip-flops
    out_q = clk && qp || !clk && qn
}

func main() {}
