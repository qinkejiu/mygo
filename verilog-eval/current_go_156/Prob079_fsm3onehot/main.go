package main

var out_next_state uint8
var out_out bool

func TopModule(in bool, state uint8) {
    // Extract one-hot bits from state (4-bit vector)
    state_A := (state & 0x01) != 0
    state_B := (state & 0x02) != 0
    state_C := (state & 0x04) != 0
    state_D := (state & 0x08) != 0

    // Compute next_state bits using one-hot transition logic
    next_A := (state_A || state_C) && !in
    next_B := (state_A || state_B || state_D) && in
    next_C := (state_B || state_D) && !in
    next_D := state_C && in

    // Pack next_state bits into uint8 (4-bit vector)
    var nextState uint8
    if next_A {
        nextState |= 0x01
    }
    if next_B {
        nextState |= 0x02
    }
    if next_C {
        nextState |= 0x04
    }
    if next_D {
        nextState |= 0x08
    }
    out_next_state = nextState

    // Output logic: out = 1 when in state D
    out_out = state_D
}

func main() {}
