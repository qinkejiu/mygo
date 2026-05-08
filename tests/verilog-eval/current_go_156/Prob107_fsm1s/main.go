package main

var out_out bool

func TopModule(clk bool, reset bool, in bool) {
    // State encoding: A=0, B=1
    var state bool = true  // Reset state is B (true)
    var next bool
    
    // Combinational logic for next state
    if state == false { // State A
        next = !in // in ? A : B -> if in=1 stay A, if in=0 go B
    } else { // State B
        next = in // in ? B : A -> if in=1 stay B, if in=0 go A
    }
    
    // Sequential logic with synchronous reset
    if clk {
        if reset {
            state = true // Reset to state B
        } else {
            state = next
        }
    }
    
    // Output logic (Moore: output depends only on state)
    out_out = state // out = (state==B)
}

func main() {}
