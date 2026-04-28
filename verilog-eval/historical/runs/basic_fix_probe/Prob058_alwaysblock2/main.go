package main

var out_assign bool
var out_always_comb bool
var out_always_ff bool

func TopModule(clk bool, a bool, b bool) {
    // assign statement equivalent
    out_assign = a != b
    
    // combinational always block equivalent
    out_always_comb = a != b
    
    // clocked always block equivalent (positive edge triggered)
    // In Go, we need to explicitly model the flip-flop behavior
    // We'll use a static variable to hold the previous XOR value
    var prevXOR bool
    if clk {
        // On positive edge of clock, update the output with current XOR
        out_always_ff = a != b
        prevXOR = a != b
    } else {
        // When clock is low, maintain the previous value
        out_always_ff = prevXOR
    }
}

func main() {}
