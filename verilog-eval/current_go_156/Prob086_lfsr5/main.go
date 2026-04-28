package main

var out_q uint8

func TopModule(clk bool, reset bool) {
    // State variable for the LFSR
    var state uint8 = 1 // Initial state after reset

    // Sequential logic triggered on positive edge of clk
    if clk {
        if reset {
            state = 1
        } else {
            // Compute next state for Galois LFSR with taps at positions 5 and 3
            // Bit positions: state[4] is MSB (bit 5), state[0] is LSB (bit 1)
            // Tap at position 5 (MSB, state[4]) and position 3 (state[2])
            
            // Save current LSB (q[0])
            lsb := state & 0x1
            
            // Shift right
            nextState := state >> 1
            
            // Apply tap at position 5 (MSB)
            if lsb != 0 {
                nextState |= 0x10 // Set bit 4 (position 5)
            }
            
            // Apply tap at position 3 (bit 2)
            if lsb != 0 {
                nextState ^= 0x04 // XOR bit 2 (position 3)
            }
            
            state = nextState
        }
    }
    
    // Assign to output global
    out_q = state & 0x1F // Ensure only 5 bits are used
}

func main() {}
