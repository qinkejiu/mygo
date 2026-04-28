package main

var out_q bool
var out_state bool

func TopModule(clk bool, a bool, b bool) {
    // Sequential element: state flip-flop
    static_c := false // This represents the previous state (flip-flop output)
    
    // On positive edge of clock, update the flip-flop
    if clk {
        // Next state logic: c <= a&b | a&c | b&c
        next_c := (a && b) || (a && static_c) || (b && static_c)
        static_c = next_c
    }
    
    // Output logic
    out_state = static_c
    out_q = (a != b) != static_c // XOR of three bits: a ^ b ^ c
}

func main() {}
