package main

var out_z bool

func TopModule(clk bool, reset bool, s bool, w bool) {
    // State encoding using uint8 (3 bits needed for 8 states)
    const (
        A   uint8 = 0
        B   uint8 = 1
        C   uint8 = 2
        S10 uint8 = 3
        S11 uint8 = 4
        S20 uint8 = 5
        S21 uint8 = 6
        S22 uint8 = 7
    )

    // State register
    var state uint8
    var next uint8

    // Sequential logic triggered on positive edge of clk
    if clk {
        if reset {
            state = A
        } else {
            state = next
        }
    }

    // Combinational next state logic
    switch state {
    case A:
        if s {
            next = B
        } else {
            next = A
        }
    case B:
        if w {
            next = S11
        } else {
            next = S10
        }
    case C:
        if w {
            next = S11
        } else {
            next = S10
        }
    case S10:
        if w {
            next = S21
        } else {
            next = S20
        }
    case S11:
        if w {
            next = S22
        } else {
            next = S21
        }
    case S20:
        next = B
    case S21:
        if w {
            next = C
        } else {
            next = B
        }
    case S22:
        if w {
            next = B
        } else {
            next = C
        }
    default:
        next = A
    }

    // Output logic
    out_z = (state == C)
}

func main() {}
