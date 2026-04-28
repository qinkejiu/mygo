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
    // Extract one-hot state bits (10 bits, using uint16 for width)
    S := (state & 0x001) != 0
    S1 := (state & 0x002) != 0
    S110 := (state & 0x008) != 0
    B0 := (state & 0x010) != 0
    B1 := (state & 0x020) != 0
    B2 := (state & 0x040) != 0
    B3 := (state & 0x080) != 0
    Count := (state & 0x100) != 0
    Wait := (state & 0x200) != 0

    // Next-state logic
    out_B3_next = B2
    out_S_next = (S && !d) || (S1 && !d) || (S110 && !d) || (Wait && ack)
    out_S1_next = S && d
    out_Count_next = B3 || (Count && !done_counting)
    out_Wait_next = (Count && done_counting) || (Wait && !ack)

    // Output logic
    out_done = Wait
    out_counting = Count
    out_shift_ena = B0 || B1 || B2 || B3
}

func main() {}
