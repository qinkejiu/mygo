package main

var out_out bool

func TopModule(clk bool, reset bool, in bool) {
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
    
    // Sequential logic with synchronous reset
    if clk {
        if reset {
            state = 0 // A
        } else {
            state = next
        }
    }
    
    // Output logic (Moore: output depends only on state)
    out_out = (state == 3) // D
}

func main() {}
