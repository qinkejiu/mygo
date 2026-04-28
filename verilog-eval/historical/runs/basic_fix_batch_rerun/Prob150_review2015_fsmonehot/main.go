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
    // Extract one-hot state bits (10 bits, using bits 0-9)
    S := (state & 0x0001) != 0
    S1 := (state & 0x0002) != 0
    S110 := (state & 0x0008) != 0
    B0 := (state & 0x0010) != 0
    B1 := (state & 0x0020) != 0
    B2 := (state & 0x0040) != 0
    B3 := (state & 0x0080) != 0
    Count := (state & 0x0100) != 0
    Wait := (state & 0x0200) != 0

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
