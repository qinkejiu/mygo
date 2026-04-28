package main

var out_start_shifting bool

func TopModule(clk bool, reset bool, data bool) {
    // State encoding
    const (
        S = iota
        S1
        S11
        S110
        Done
    )
    
    // State register
    var state uint8 = S
    var next uint8 = S
    
    // Combinational next state logic
    switch state {
    case S:
        if data {
            next = S1
        } else {
            next = S
        }
    case S1:
        if data {
            next = S11
        } else {
            next = S
        }
    case S11:
        if data {
            next = S11
        } else {
            next = S110
        }
    case S110:
        if data {
            next = Done
        } else {
            next = S
        }
    case Done:
        next = Done
    }
    
    // Sequential logic (positive edge triggered)
    if clk {
        if reset {
            state = S
        } else {
            state = next
        }
    }
    
    // Output logic
    out_start_shifting = state == Done
}

func main() {}
