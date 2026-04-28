package main

var out_g0 bool
var out_g1 bool
var out_g2 bool

func TopModule(clk bool, resetn bool, r uint8) {
    // State encoding: A=0, B=1, C=2, D=3
    var state uint8
    var next uint8

    // State transition logic (combinational)
    switch state {
    case 0: // State A
        if (r & 0x01) != 0 { // r[0] == 1
            next = 1 // B
        } else if (r & 0x02) != 0 { // r[1] == 1
            next = 2 // C
        } else if (r & 0x04) != 0 { // r[2] == 1
            next = 3 // D
        } else {
            next = 0 // A
        }
    case 1: // State B
        if (r & 0x01) != 0 { // r[0] == 1
            next = 1 // B
        } else {
            next = 0 // A
        }
    case 2: // State C
        if (r & 0x02) != 0 { // r[1] == 1
            next = 2 // C
        } else {
            next = 0 // A
        }
    case 3: // State D
        if (r & 0x04) != 0 { // r[2] == 1
            next = 3 // D
        } else {
            next = 0 // A
        }
    default:
        next = 0 // A (safe default)
    }

    // Sequential logic (positive edge triggered)
    if clk {
        if !resetn { // active-low synchronous reset
            state = 0 // A
        } else {
            state = next
        }
    }

    // Output logic (combinational)
    out_g0 = (state == 1) // B
    out_g1 = (state == 2) // C
    out_g2 = (state == 3) // D
}

func main() {}
