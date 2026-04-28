package main

var out_bytes uint32
var out_done bool

func TopModule(clk bool, reset bool, in uint8) {
    // State encoding
    const (
        BYTE1 = 0
        BYTE2 = 1
        BYTE3 = 2
        DONE  = 3
    )

    // State register
    var state uint8
    var nextState uint8

    // Shift register for storing the 3 bytes
    var shiftReg uint32

    // Check in[3] (bit 3)
    in3 := (in & 0x08) != 0

    // Next state logic
    switch state {
    case BYTE1:
        if in3 {
            nextState = BYTE2
        } else {
            nextState = BYTE1
        }
    case BYTE2:
        nextState = BYTE3
    case BYTE3:
        nextState = DONE
    case DONE:
        if in3 {
            nextState = BYTE2
        } else {
            nextState = BYTE1
        }
    }

    // Clocked logic
    if clk {
        if reset {
            state = BYTE1
            shiftReg = 0
        } else {
            state = nextState
            
            // Shift in new byte (shiftReg[23:16] is first byte)
            shiftReg = (shiftReg << 8) | uint32(in)
        }
    }

    // Output logic
    out_done = (state == DONE)
    
    if out_done {
        out_bytes = shiftReg & 0x00FFFFFF // Ensure only 24 bits
    } else {
        // Don't-care when done is not asserted
        out_bytes = 0
    }
}

func main() {}
