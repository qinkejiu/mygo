package main

var out_z bool

func TopModule(clk bool, aresetn bool, x bool) {
    // State encoding: 0=S, 1=S1, 2=S10
    var state uint8
    var next uint8
    
    // Sequential logic with asynchronous reset
    if !aresetn {
        state = 0
    } else if clk {
        state = next
    }
    
    // Combinational next state logic
    switch state {
    case 0: // S
        if x {
            next = 1 // S1
        } else {
            next = 0 // S
        }
    case 1: // S1
        if x {
            next = 1 // S1
        } else {
            next = 2 // S10
        }
    case 2: // S10
        if x {
            next = 1 // S1
        } else {
            next = 0 // S
        }
    default:
        next = 0
    }
    
    // Combinational output logic (Mealy)
    switch state {
    case 0: // S
        out_z = false
    case 1: // S1
        out_z = false
    case 2: // S10
        out_z = x
    default:
        out_z = false
    }
}

func main() {}
