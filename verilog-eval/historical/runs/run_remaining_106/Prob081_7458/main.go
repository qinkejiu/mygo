package main

var out_p1y bool
var out_p2y bool

func TopModule(p1a bool, p1b bool, p1c bool, p1d bool, p1e bool, p1f bool, p2a bool, p2b bool, p2c bool, p2d bool) {
    // First 3-input AND: p1a AND p1b AND p1c
    and1 := p1a && p1b && p1c
    
    // Second 3-input AND: p1d AND p1e AND p1f
    and2 := p1d && p1e && p1f
    
    // p1y is OR of the two 3-input AND gates
    out_p1y = and1 || and2
    
    // First 2-input AND: p2a AND p2b
    and3 := p2a && p2b
    
    // Second 2-input AND: p2c AND p2d
    and4 := p2c && p2d
    
    // p2y is OR of the two 2-input AND gates
    out_p2y = and3 || and4
}

func main() {}
