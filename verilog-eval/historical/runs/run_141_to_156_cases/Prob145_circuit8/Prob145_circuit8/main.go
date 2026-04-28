package main

var out_p bool
var out_q bool

func TopModule(clock bool, a bool) {
    // Sequential logic for q (negative edge triggered)
    if !clock {
        out_q = a
    }
    
    // Combinational logic for p (clock high)
    if clock {
        out_p = a
    }
}

func main() {}
