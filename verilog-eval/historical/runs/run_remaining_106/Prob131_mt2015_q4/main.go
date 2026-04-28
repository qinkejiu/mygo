package main

var out_z bool

func TopModule(x bool, y bool) {
    // Module A: z = (x^y) & x
    a1_out := (x != y) && x
    a2_out := (x != y) && x
    
    // Module B: Based on waveform analysis, z = x|~y
    b1_out := x || !y
    b2_out := x || !y
    
    // First OR: a1_out OR b1_out
    or_out := a1_out || b1_out
    
    // Second AND: a2_out AND b2_out
    and_out := a2_out && b2_out
    
    // Final XOR: or_out XOR and_out
    out_z = or_out != and_out
}

func main() {}
