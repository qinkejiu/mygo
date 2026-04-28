package main

var out_walk_left bool
var out_walk_right bool
var out_aaah bool
var out_digging bool

func TopModule(clk bool, areset bool, bump_left bool, bump_right bool, ground bool, dig bool) {
    // State encoding constants
    const (
        WL = 0 // Walking left
        WR = 1 // Walking right
        FALLL = 2 // Falling left
        FALLR = 3 // Falling right
        DIGL = 4 // Digging left
        DIGR = 5 // Digging right
    )
    
    // State register (3 bits to hold values 0-5)
    var state uint8 = 0
    var next uint8 = 0
    
    // Sequential logic (simulated with conditionals)
    if areset {
        state = WL
    } else if clk {
        // Positive edge triggered
        state = next
    }
    
    // Combinational next state logic
    switch state {
    case WL:
        if !ground {
            next = FALLL
        } else if dig {
            next = DIGL
        } else if bump_left {
            next = WR
        } else {
            next = WL
        }
    case WR:
        if !ground {
            next = FALLR
        } else if dig {
            next = DIGR
        } else if bump_right {
            next = WL
        } else {
            next = WR
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
    case DIGL:
        if ground {
            next = DIGL
        } else {
            next = FALLL
        }
    case DIGR:
        if ground {
            next = DIGR
        } else {
            next = FALLR
        }
    default:
        next = WL // Default to walking left
    }
    
    // Output logic (Moore outputs)
    out_walk_left = (state == WL)
    out_walk_right = (state == WR)
    out_aaah = (state == FALLL) || (state == FALLR)
    out_digging = (state == DIGL) || (state == DIGR)
}

func main() {}
