package main

var out_q bool
var out_state bool

func TopModule(clk bool, a bool, b bool) {
    // Combinational logic for next state
    next_state := (a && b) || (a && out_state) || (b && out_state)
    
    // Update state on positive clock edge
    if clk {
        out_state = next_state
    }
    
    // Combinational output q
    out_q = (a != b) != out_state
}

func main() {}
