package main

var out_q uint8

func TopModule(clk bool, a bool) {
    // Internal state variable to hold the current value of q
    var state uint8 = 0
    
    // On positive edge of clk
    if clk {
        if a {
            // When a is high, set q to 4
            state = 4
        } else if state == 6 {
            // When a is low and q is 6, wrap around to 0
            state = 0
        } else {
            // Otherwise increment q
            state = state + 1
        }
    }
    
    // Assign to output global
    out_q = state & 0x7 // Mask to keep only 3 bits
}

func main() {}
