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
    // Extract one-hot state bits (only 10 bits needed, but state is uint16)
    // Bit positions: S=0, S1=1, S11=2, S110=3, B0=4, B1=5, B2=6, B3=7, Count=8, Wait=9
    state_S := (state & 0x0001) != 0
    state_S1 := (state & 0x0002) != 0
    state_S11 := (state & 0x0004) != 0
    state_S110 := (state & 0x0008) != 0
    state_B0 := (state & 0x0010) != 0
    state_B1 := (state & 0x0020) != 0
    state_B2 := (state & 0x0040) != 0
    state_B3 := (state & 0x0080) != 0
    state_Count := (state & 0x0100) != 0
    state_Wait := (state & 0x0200) != 0

    // Next-state logic
    out_B3_next = state_B2
    out_S_next = (state_S && !d) || (state_S1 && !d) || (state_S110 && !d) || (state_Wait && ack)
    out_S1_next = state_S && d
    out_Count_next = state_B3 || (state_Count && !done_counting)
    out_Wait_next = (state_Count && done_counting) || (state_Wait && !ack)

    // Output logic
    out_done = state_Wait
    out_counting = state_Count
    out_shift_ena = state_B0 || state_B1 || state_B2 || state_B3
}

func main() {}
