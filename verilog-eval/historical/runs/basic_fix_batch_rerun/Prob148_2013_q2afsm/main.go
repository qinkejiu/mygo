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
        if (r & 0x1) != 0 { // r[0] == 1
            next = 1
        } else if (r & 0x2) != 0 { // r[1] == 1
            next = 2
        } else if (r & 0x4) != 0 { // r[2] == 1
            next = 3
        } else {
            next = 0
        }
    case 1: // State B
        if (r & 0x1) != 0 { // r[0] == 1
            next = 1
        } else {
            next = 0
        }
    case 2: // State C
        if (r & 0x2) != 0 { // r[1] == 1
            next = 2
        } else {
            next = 0
        }
    case 3: // State D
        if (r & 0x4) != 0 { // r[2] == 1
            next = 3
        } else {
            next = 0
        }
    default:
        next = 0
    }

    // State register (sequential)
    if clk {
        if !resetn {
            state = 0
        } else {
            state = next
        }
    }

    // Output logic (combinational)
    out_g0 = (state == 1)
    out_g1 = (state == 2)
    out_g2 = (state == 3)
}

func main() {}
