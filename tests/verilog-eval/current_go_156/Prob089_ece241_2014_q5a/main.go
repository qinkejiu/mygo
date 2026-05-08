package main

var out_z bool

func TopModule(clk bool, areset bool, x bool) {
    // State encoding: A=0, B=1, C=2
    // Use uint8 for state (2 bits needed)
    var state uint8
    
    // Sequential logic triggered on positive edge of clk
    if areset {
        // Asynchronous reset
        state = 0 // State A
    } else if clk {
        // Positive edge triggered logic
        switch state {
        case 0: // State A
            if x {
                state = 2 // State C
            } else {
                state = 0 // State A
            }
        case 1: // State B
            if x {
                state = 1 // State B
            } else {
                state = 2 // State C
            }
        case 2: // State C
            if x {
                state = 1 // State B
            } else {
                state = 2 // State C
            }
        }
    }
    
    // Output logic: z = (state == C)
    out_z = (state == 2)
}

func main() {}
