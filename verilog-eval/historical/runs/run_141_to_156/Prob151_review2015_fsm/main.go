package main

var out_shift_ena bool
var out_counting bool
var out_done bool

func TopModule(clk bool, reset bool, data bool, done_counting bool, ack bool) {
    // State encoding
    const (
        S = iota
        S1
        S11
        S110
        B0
        B1
        B2
        B3
        Count
        Wait
    )

    // State register
    var state uint8
    var next uint8

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
            next = B0
        } else {
            next = S
        }
    case B0:
        next = B1
    case B1:
        next = B2
    case B2:
        next = B3
    case B3:
        next = Count
    case Count:
        if done_counting {
            next = Wait
        } else {
            next = Count
        }
    case Wait:
        if ack {
            next = S
        } else {
            next = Wait
        }
    default:
        next = S
    }

    // Sequential logic (positive edge triggered, synchronous reset)
    if reset {
        state = S
    } else if clk {
        state = next
    }

    // Output logic
    out_shift_ena = (state == B0) || (state == B1) || (state == B2) || (state == B3)
    out_counting = (state == Count)
    out_done = (state == Wait)
}

func main() {}
