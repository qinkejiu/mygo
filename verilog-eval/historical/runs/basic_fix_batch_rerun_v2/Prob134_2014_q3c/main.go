package main

var out_Y0 bool
var out_z bool

func TopModule(clk bool, x bool, y uint8) {
    // Extract y bits (3-bit input)
    y0 := (y & 0x1) != 0
    y1 := (y & 0x2) != 0
    y2 := (y & 0x4) != 0

    // Determine next state Y[0] based on present state and x
    var nextY0 bool

    // Present state 000
    if !y2 && !y1 && !y0 {
        if !x {
            // x=0: next state 000
            nextY0 = false
        } else {
            // x=1: next state 001
            nextY0 = true
        }
    }

    // Present state 001
    if !y2 && !y1 && y0 {
        if !x {
            // x=0: next state 001
            nextY0 = true
        } else {
            // x=1: next state 100
            nextY0 = false
        }
    }

    // Present state 010
    if !y2 && y1 && !y0 {
        if !x {
            // x=0: next state 010
            nextY0 = false
        } else {
            // x=1: next state 001
            nextY0 = true
        }
    }

    // Present state 011
    if !y2 && y1 && y0 {
        if !x {
            // x=0: next state 001
            nextY0 = true
        } else {
            // x=1: next state 010
            nextY0 = false
        }
    }

    // Present state 100
    if y2 && !y1 && !y0 {
        if !x {
            // x=0: next state 011
            nextY0 = true
        } else {
            // x=1: next state 100
            nextY0 = false
        }
    }

    // Output Y0 is Y[0] of the next state signal
    out_Y0 = nextY0

    // Output z based on present state (from table)
    switch y {
    case 0x0, 0x1, 0x2:
        out_z = false
    case 0x3, 0x4:
        out_z = true
    default:
        out_z = false
    }
}

func main() {}
