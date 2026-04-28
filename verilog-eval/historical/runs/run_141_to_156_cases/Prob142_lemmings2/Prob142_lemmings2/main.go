package main

var out_walk_left bool
var out_walk_right bool
var out_aaah bool

func TopModule(clk bool, areset bool, bump_left bool, bump_right bool, ground bool) {
    // State encoding
    // WL = 0, WR = 1, FALLL = 2, FALLR = 3
    var state uint8
    var next uint8

    // Next state logic
    switch state {
    case 0: // WL
        if ground {
            if bump_left {
                next = 1 // WR
            } else {
                next = 0 // WL
            }
        } else {
            next = 2 // FALLL
        }
    case 1: // WR
        if ground {
            if bump_right {
                next = 0 // WL
            } else {
                next = 1 // WR
            }
        } else {
            next = 3 // FALLR
        }
    case 2: // FALLL
        if ground {
            next = 0 // WL
        } else {
            next = 2 // FALLL
        }
    case 3: // FALLR
        if ground {
            next = 1 // WR
        } else {
            next = 3 // FALLR
        }
    }

    // Sequential logic with asynchronous reset
    if areset {
        state = 0 // WL
    } else if clk {
        state = next
    }

    // Output logic (Moore)
    out_walk_left = (state == 0)
    out_walk_right = (state == 1)
    out_aaah = (state == 2) || (state == 3)
}

func main() {}
