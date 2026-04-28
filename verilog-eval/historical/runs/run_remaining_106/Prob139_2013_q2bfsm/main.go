package main

var out_f bool
var out_g bool

func TopModule(clk bool, resetn bool, x bool, y bool) {
    // State encoding
    const (
        A   = 0
        B   = 1
        S0  = 2
        S1  = 3
        S10 = 4
        G1  = 5
        G2  = 6
        P0  = 7
        P1  = 8
    )

    // State register (4 bits to hold values 0-8)
    var state uint8
    var next uint8

    // Sequential logic
    if !resetn {
        state = A
    } else if clk {
        state = next
    }

    // Combinational next-state logic
    switch state {
    case A:
        next = B
    case B:
        next = S0
    case S0:
        if x {
            next = S1
        } else {
            next = S0
        }
    case S1:
        if x {
            next = S1
        } else {
            next = S10
        }
    case S10:
        if x {
            next = G1
        } else {
            next = S0
        }
    case G1:
        if y {
            next = P1
        } else {
            next = G2
        }
    case G2:
        if y {
            next = P1
        } else {
            next = P0
        }
    case P0:
        next = P0
    case P1:
        next = P1
    default:
        next = A
    }

    // Output logic
    out_f = (state == B)
    out_g = (state == G1) || (state == G2) || (state == P1)
}

func main() {}
