package main

var out_z bool

func TopModule(clk bool, areset bool, x bool) {
    // One-hot state encoding: state_A = true, state_B = false
    // We'll use a bool for state_A (true means in state A, false means in state B)
    var state_A bool
    
    // Asynchronous reset
    if areset {
        state_A = true
    } else if clk {
        // Positive edge triggered logic
        // Next state logic
        if state_A {
            // State A
            if x {
                state_A = false // Go to state B
            }
            // else stay in state A (state_A remains true)
        }
        // State B always stays in state B (state_A remains false)
    }
    
    // Output logic (Mealy)
    out_z = (state_A && x) || (!state_A && !x)
}

func main() {}
