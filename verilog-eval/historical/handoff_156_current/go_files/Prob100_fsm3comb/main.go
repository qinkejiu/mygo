package main

var out_next_state uint8
var out_out bool

func TopModule(in bool, state uint8) {
    // State encoding: A=0, B=1, C=2, D=3
    switch state {
    case 0: // A
        if in {
            out_next_state = 1 // B
        } else {
            out_next_state = 0 // A
        }
    case 1: // B
        if in {
            out_next_state = 1 // B
        } else {
            out_next_state = 2 // C
        }
    case 2: // C
        if in {
            out_next_state = 3 // D
        } else {
            out_next_state = 0 // A
        }
    case 3: // D
        if in {
            out_next_state = 1 // B
        } else {
            out_next_state = 2 // C
        }
    default:
        out_next_state = 0 // Default to A
    }

    // Output is 1 only when current state is D (3)
    out_out = (state == 3)
}

func main() {}
