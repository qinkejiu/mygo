package main

var out_assign bool
var out_always_comb bool
var out_always_ff bool
var ff_state_storage bool

func TopModule(clk bool, a bool, b bool) {
    out_assign = a != b
    out_always_comb = a != b
    if clk {
        ff_state_storage = a != b
    }
    out_always_ff = ff_state_storage
}

func main() {}
