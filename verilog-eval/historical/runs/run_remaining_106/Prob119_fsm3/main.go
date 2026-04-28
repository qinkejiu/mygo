package main

var out_out bool

func TopModule(clk bool, areset bool, in bool) {
    // State encoding: A=0, B=1, C=2, D=3
    var state uint8
    var next uint8
    
    // Combinational next state logic
    switch state {
    case 0: // A
        if in {
            next = 1 // B
        } else {
            next = 0 // A
        }
    case 1: // B
        if in {
            next = 1 // B
        } else {
            next = 2 // C
        }
    case 2: // C
        if in {
            next = 3 // D
        } else {
            next = 0 // A
        }
    case 3: // D
        if in {
            next = 1 // B
        } else {
            next = 2 // C
        }
    }
    
    // Sequential logic with positive edge triggered clock and asynchronous reset
    // In Go, we model this with conditionals
    if areset {
        state = 0 // Reset to state A
    } else if clk {
        // Positive edge triggered - update state on clock edge
        state = next
    }
    
    // Output logic (Moore: output depends only on current state)
    out_out = (state == 3) // Output 1 when in state D
}

func main() {}
