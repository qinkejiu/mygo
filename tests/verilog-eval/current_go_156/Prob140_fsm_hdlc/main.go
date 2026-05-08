package main

var out_disc bool
var out_flag bool
var out_err bool

func TopModule(clk bool, reset bool, in bool) {
    // State encoding
    const (
        S0 = uint8(0)
        S1 = uint8(1)
        S2 = uint8(2)
        S3 = uint8(3)
        S4 = uint8(4)
        S5 = uint8(5)
        S6 = uint8(6)
        SERR = uint8(7)
        SDISC = uint8(8)
        SFLAG = uint8(9)
    )

    // State register
    var state uint8 = S0
    var next uint8

    // State transition logic
    switch state {
    case S0:
        if in {
            next = S1
        } else {
            next = S0
        }
    case S1:
        if in {
            next = S2
        } else {
            next = S0
        }
    case S2:
        if in {
            next = S3
        } else {
            next = S0
        }
    case S3:
        if in {
            next = S4
        } else {
            next = S0
        }
    case S4:
        if in {
            next = S5
        } else {
            next = S0
        }
    case S5:
        if in {
            next = S6
        } else {
            next = SDISC
        }
    case S6:
        if in {
            next = SERR
        } else {
            next = SFLAG
        }
    case SERR:
        if in {
            next = SERR
        } else {
            next = S0
        }
    case SFLAG:
        if in {
            next = S1
        } else {
            next = S0
        }
    case SDISC:
        if in {
            next = S1
        } else {
            next = S0
        }
    default:
        next = S0
    }

    // Clocked behavior
    if clk {
        if reset {
            state = S0
        } else {
            state = next
        }
    }

    // Output logic (Moore type)
    out_disc = (state == SDISC)
    out_flag = (state == SFLAG)
    out_err = (state == SERR)
}

func main() {}
