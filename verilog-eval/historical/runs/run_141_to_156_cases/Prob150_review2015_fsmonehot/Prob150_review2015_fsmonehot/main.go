package main

var out_B3_next bool
var out_S_next bool
var out_S1_next bool
var out_Count_next bool
var out_Wait_next bool
var out_done bool
var out_counting bool
var out_shift_ena bool

func TopModule(d bool, done_counting bool, ack bool, state uint16) {
    // Extract one-hot state bits (10 bits, using uint16 for width up to 16 bits)
    // state[0] = S, state[1] = S1, state[2] = S11, state[3] = S110,
    // state[4] = B0, state[5] = B1, state[6] = B2, state[7] = B3,
    // state[8] = Count, state[9] = Wait
    s_state := (state & 0x001) != 0
    s1_state := (state & 0x002) != 0
    s11_state := (state & 0x004) != 0
    s110_state := (state & 0x008) != 0
    b0_state := (state & 0x010) != 0
    b1_state := (state & 0x020) != 0
    b2_state := (state & 0x040) != 0
    b3_state := (state & 0x080) != 0
    count_state := (state & 0x100) != 0
    wait_state := (state & 0x200) != 0

    // Next-state logic
    out_B3_next = b2_state
    out_S_next = (s_state && !d) || (s1_state && !d) || (s110_state && !d) || (wait_state && ack)
    out_S1_next = s_state && d
    out_Count_next = b3_state || (count_state && !done_counting)
    out_Wait_next = (count_state && done_counting) || (wait_state && !ack)

    // Output logic
    out_done = wait_state
    out_counting = count_state
    out_shift_ena = b0_state || b1_state || b2_state || b3_state
}

func main() {}
