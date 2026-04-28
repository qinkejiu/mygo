package main

var out_q uint8

func TopModule(clk bool, reset bool) {
    // Internal state for the counter
    var counter uint8 = 0
    
    // Sequential logic triggered on positive edge of clk
    if clk {
        if reset {
            counter = 0
        } else {
            // Increment counter, wrapping at 15
            counter = (counter + 1) & 0x0F
        }
    }
    
    // Assign to output global
    out_q = counter
}

func main() {}
