package main

var out_walk_left bool
var out_walk_right bool
var out_aaah bool
var out_digging bool

func TopModule(clk bool, areset bool, bump_left bool, bump_right bool, ground bool, dig bool) {
    // State encoding
    const (
        WL = uint8(0) // walking left
        WR = uint8(1) // walking right
        FALLL = uint8(2) // falling left
        FALLR = uint8(3) // falling right
        DIGL = uint8(4) // digging left
        DIGR = uint8(5) // digging right
        DEAD = uint8(6) // dead
    )
    
    // State register and fall counter
    var state uint8
    var fall_counter uint8
    
    // Next state logic
    var next_state uint8
    
    // Combinational logic for next state
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
            if fall_counter >= 20 {
                next_state = DEAD
            } else {
                next_state = WL
            }
        } else {
            next_state = FALLL
        }
    case FALLR:
        if ground {
            if fall_counter >= 20 {
                next_state = DEAD
            } else {
                next_state = WR
            }
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
    case DEAD:
        next_state = DEAD
    }
    
    // Sequential logic (clock edge)
    if areset {
        state = WL
        fall_counter = 0
    } else if clk {
        state = next_state
        
        // Update fall counter
        if state == FALLL || state == FALLR {
            if fall_counter < 20 {
                fall_counter++
            }
        } else {
            fall_counter = 0
        }
    }
    
    // Output logic (Moore outputs)
    out_walk_left = (state == WL)
    out_walk_right = (state == WR)
    out_aaah = (state == FALLL) || (state == FALLR)
    out_digging = (state == DIGL) || (state == DIGR)
}

func main() {}
