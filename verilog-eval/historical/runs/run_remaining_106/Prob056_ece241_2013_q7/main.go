package main

var out_Q bool

func TopModule(clk bool, j bool, k bool) {
    // Static variable to hold the state across calls
    var Qold bool = false

    // Check for positive edge of clock
    // In hardware simulation, we assume this function is called every cycle
    // and clk transitions are detected by comparing with previous state
    static var prev_clk bool = false
    if clk && !prev_clk {
        // Positive edge detected - update flip-flop
        if !j && !k {
            // J=0, K=0: Q remains Qold
            out_Q = Qold
        } else if !j && k {
            // J=0, K=1: Q becomes 0
            out_Q = false
        } else if j && !k {
            // J=1, K=0: Q becomes 1
            out_Q = true
        } else { // j && k
            // J=1, K=1: Q becomes ~Qold
            out_Q = !Qold
        }
        // Update Qold for next cycle
        Qold = out_Q
    } else {
        // No positive edge - maintain current output
        out_Q = Qold
    }
    
    // Update previous clock state for next call
    prev_clk = clk
}

func main() {}
