package main

var out_next_state uint16
var out_out1 bool
var out_out2 bool

func TopModule(in bool, state uint16) {
    // Output logic
    out_out1 = (state&(1<<8)) != 0 || (state&(1<<9)) != 0
    out_out2 = (state&(1<<7)) != 0 || (state&(1<<9)) != 0

    // Next state logic
    next_state := uint16(0)

    // next_state[0] = !in && (|state[4:0] | state[7] | state[8] | state[9])
    if !in {
        // Check if any of bits 0-4, 7, 8, or 9 are set
        if (state&0x1F) != 0 || (state&(1<<7)) != 0 || (state&(1<<8)) != 0 || (state&(1<<9)) != 0 {
            next_state |= 1 << 0
        }
    }

    // next_state[1] = in && (state[0] | state[8] | state[9])
    if in && ((state&(1<<0)) != 0 || (state&(1<<8)) != 0 || (state&(1<<9)) != 0) {
        next_state |= 1 << 1
    }

    // next_state[2] = in && state[1]
    if in && (state&(1<<1)) != 0 {
        next_state |= 1 << 2
    }

    // next_state[3] = in && state[2]
    if in && (state&(1<<2)) != 0 {
        next_state |= 1 << 3
    }

    // next_state[4] = in && state[3]
    if in && (state&(1<<3)) != 0 {
        next_state |= 1 << 4
    }

    // next_state[5] = in && state[4]
    if in && (state&(1<<4)) != 0 {
        next_state |= 1 << 5
    }

    // next_state[6] = in && state[5]
    if in && (state&(1<<5)) != 0 {
        next_state |= 1 << 6
    }

    // next_state[7] = in && (state[6] | state[7])
    if in && ((state&(1<<6)) != 0 || (state&(1<<7)) != 0) {
        next_state |= 1 << 7
    }

    // next_state[8] = !in && state[5]
    if !in && (state&(1<<5)) != 0 {
        next_state |= 1 << 8
    }

    // next_state[9] = !in && state[6]
    if !in && (state&(1<<6)) != 0 {
        next_state |= 1 << 9
    }

    out_next_state = next_state
}

func main() {}
