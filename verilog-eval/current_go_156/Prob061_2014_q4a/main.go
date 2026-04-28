package main

var out_Q bool

func TopModule(clk bool, w bool, R bool, E bool, L bool) {
    // Sequential logic triggered on positive edge of clk
    // In MyGO, we model this with an explicit clock check
    if clk {
        if L {
            out_Q = R
        } else if E {
            out_Q = w
        }
        // Note: In real hardware, Q would retain its value when neither L nor E is true
        // This is implicit in the register behavior, but in this Go model,
        // we need to ensure out_Q isn't changed when conditions aren't met.
        // Since out_Q is a package variable, it persists between calls.
    }
}

func main() {}
