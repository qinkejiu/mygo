package main

var out_walk_left bool
var out_walk_right bool
var out_aaah bool
var out_digging bool

func TopModule(clk bool, areset bool, bump_left bool, bump_right bool, ground bool, dig bool) {
    // State encoding constants
    const (
        WL = uint8(0) // Walk left
        WR = uint8(1) // Walk right
        FALLL = uint8(2) // Fall left
        FALLR = uint8(3) // Fall right
        DIGL = uint8(4) // Dig left
        DIGR = uint8(5) // Dig right
    )

    // State register
    var state uint8
    var next_state uint8

    // Sequential logic
    if areset {
        state = WL
    } else if clk {
        state = next_state
    }

    // Combinational next state logic
    switch state {
    case WL:
        if !ground {
            next_state = FALLL
        } else if dig {
            next_state = DIGL
        } else if bump_left {
            next_state = WR
        } else {
            next_state = WL
        }
    case WR:
        if !ground {
            next_state = FALLR
        } else if dig {
            next_state = DIGR
        } else if bump_right {
            next_state = WL
        } else {
            next_state = WR
        }
    case FALLL:
        if ground {
            next_state = WL
        } else {
            next_state = FALLL
        }
    case FALLR:
        if ground {
            next_state = WR
        } else {
            next_state = FALLR
        }
    case DIGL:
        if ground {
            next_state = DIGL
        } else {
            next_state = FALLL
        }
    case DIGR:
        if ground {
            next_state = DIGR
        } else {
            next_state = FALLR
        }
    default:
        next_state = WL
    }

    // Output logic (Moore outputs)
    out_walk_left = (state == WL)
    out_walk_right = (state == WR)
    out_aaah = (state == FALLL) || (state == FALLR)
    out_digging = (state == DIGL) || (state == DIGR)
}

func main() {}
