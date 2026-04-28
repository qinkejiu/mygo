package main

var out_out bool

func TopModule(clk bool, reset bool, j bool, k bool) {
    // State encoding: false = OFF (A), true = ON (B)
    var state bool
    var next bool
    
    // Combinational next state logic
    if !state { // OFF state
        if j {
            next = true // ON
        } else {
            next = false // OFF
        }
    } else { // ON state
        if k {
            next = false // OFF
        } else {
            next = true // ON
        }
    }
    
    // Sequential logic with synchronous reset
    if clk {
        if reset {
            state = false // Reset to OFF
        } else {
            state = next
        }
    }
    
    // Output logic (Moore: output depends only on state)
    out_out = state // true when ON, false when OFF
}

func main() {}
