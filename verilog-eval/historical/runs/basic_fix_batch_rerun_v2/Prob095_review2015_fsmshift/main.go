package main

var out_shift_ena bool

func TopModule(clk bool, reset bool) {
    // State encoding: B0=0, B1=1, B2=2, B3=3, Done=4
    // Use uint8 for state (3 bits needed, but uint8 is smallest unsigned type)
    var state uint8
    var next uint8
    
    // Combinational logic for next state
    switch state {
    case 0: // B0
        next = 1
    case 1: // B1
        next = 2
    case 2: // B2
        next = 3
    case 3: // B3
        next = 4
    case 4: // Done
        next = 4
    default:
        next = 4
    }
    
    // Sequential logic (positive edge triggered)
    // In MyGO, we model this with conditionals on clk
    // Since clk is a bool input, we check if we're at a positive edge
    // For simulation purposes, we'll update state when clk is true
    if clk {
        if reset {
            state = 0
        } else {
            state = next
        }
    }
    
    // Output logic: shift_ena is true for states B0 through B3
    out_shift_ena = (state == 0 || state == 1 || state == 2 || state == 3)
}

func main() {}
