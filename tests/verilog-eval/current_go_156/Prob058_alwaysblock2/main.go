package main

var out_assign bool
var out_always_comb bool
var out_always_ff bool

func TopModule(clk bool, a bool, b bool) {
    // assign statement equivalent - combinational XOR
    out_assign = a != b
    
    // combinational always block equivalent - also combinational XOR
    out_always_comb = a != b
    
    // clocked always block equivalent - sequential with flip-flop
    // In real hardware, this would only update on posedge clk
    // We model this by checking if we're at a positive edge
    // For simplicity in this model, we'll assume the function is called
    // at each clock cycle and we update on positive edge
    if clk {
        // On positive edge of clock, update the flip-flop output
        out_always_ff = a != b
    }
    // Note: In a real simulation, we'd need to track previous clock state
    // to detect edges. This simplified model assumes TopModule is called
    // once per clock cycle with the current clock value.
}

func main() {}
