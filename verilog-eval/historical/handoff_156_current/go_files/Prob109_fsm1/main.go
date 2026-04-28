package main

var out_out bool

func TopModule(clk bool, areset bool, in bool) {
    // State encoding: A=0, B=1
    var state bool
    var next bool
    
    // Combinational logic for next state
    if state == false { // State A
        if in {
            next = false // A -> A
        } else {
            next = true // A -> B
        }
    } else { // State B
        if in {
            next = true // B -> B
        } else {
            next = false // B -> A
        }
    }
    
    // Sequential logic with asynchronous reset
    if areset {
        state = true // Reset to state B
    } else if clk {
        state = next
    }
    
    // Output logic (Moore: output depends only on state)
    out_out = state // out = 1 when in state B
}

func main() {}
