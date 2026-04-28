package main

var out_z bool

func TopModule(clk bool, x bool) {
    // D flip-flops with initial reset to zero
    var s0, s1, s2 bool
    
    // Sequential logic triggered on positive edge of clk
    if clk {
        // XOR gate: s2 ^ x
        s2 = (s2 != x)
        
        // AND gate: ~s1 & x
        s1 = (!s1 && x)
        
        // OR gate: ~s0 | x
        s0 = (!s0 || x)
    }
    
    // Three-input NOR gate: ~|s
    out_z = !(s0 || s1 || s2)
}

func main() {}
