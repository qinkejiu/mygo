package main

var out_walk_left bool
var out_walk_right bool
var out_aaah bool

func TopModule(clk bool, areset bool, bump_left bool, bump_right bool, ground bool) {
    // State encoding
    const (
        WL = 0 // 00: walking left
        WR = 1 // 01: walking right
        FALLL = 2 // 10: falling left
        FALLR = 3 // 11: falling right
    )
    
    // State register (2 bits)
    var state uint8
    var next uint8
    
    // Sequential logic
    if areset {
        state = WL
    } else if clk {
        state = next
    }
    
    // Combinational next state logic
    switch state {
    case WL:
        if ground {
            if bump_left {
                next = WR
            } else {
                next = WL
            }
        } else {
            next = FALLL
        }
    case WR:
        if ground {
            if bump_right {
                next = WL
            } else {
                next = WR
            }
        } else {
            next = FALLR
        }
    case FALLL:
        if ground {
            next = WL
        } else {
            next = FALLL
        }
    case FALLR:
        if ground {
            next = WR
        } else {
            next = FALLR
        }
    }
    
    // Output logic (Moore outputs)
    out_walk_left = (state == WL)
    out_walk_right = (state == WR)
    out_aaah = (state == FALLL) || (state == FALLR)
}

func main() {}
