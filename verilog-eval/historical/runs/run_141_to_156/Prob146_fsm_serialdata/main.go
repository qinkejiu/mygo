package main

var out_byte uint8
var out_done bool

func TopModule(clk bool, in bool, reset bool) {
    // State encoding
    const (
        START = 0
        B0    = 1
        B1    = 2
        B2    = 3
        B3    = 4
        B4    = 5
        B5    = 6
        B6    = 7
        B7    = 8
        STOP  = 9
        DONE  = 10
        ERR   = 11
    )

    // State register
    var state uint8 = START
    var next_state uint8

    // Shift register for capturing bits (10 bits: start + 8 data + stop)
    var byte_shift uint16

    // Determine next state (combinational logic)
    switch state {
    case START:
        if in {
            next_state = START
        } else {
            next_state = B0
        }
    case B0:
        next_state = B1
    case B1:
        next_state = B2
    case B2:
        next_state = B3
    case B3:
        next_state = B4
    case B4:
        next_state = B5
    case B5:
        next_state = B6
    case B6:
        next_state = B7
    case B7:
        next_state = STOP
    case STOP:
        if in {
            next_state = DONE
        } else {
            next_state = ERR
        }
    case DONE:
        if in {
            next_state = START
        } else {
            next_state = B0
        }
    case ERR:
        if in {
            next_state = START
        } else {
            next_state = ERR
        }
    default:
        next_state = START
    }

    // Sequential logic triggered on positive edge of clk
    if clk {
        if reset {
            state = START
            byte_shift = 0
        } else {
            state = next_state

            // Shift in new bit at LSB position
            if in {
                byte_shift = (byte_shift >> 1) | (1 << 9)
            } else {
                byte_shift = (byte_shift >> 1) & ^(1 << 9)
            }
        }
    }

    // Output logic
    out_done = (state == DONE)
    if out_done {
        // Extract bits 8:1 from the shift register (data bits)
        out_byte = uint8((byte_shift >> 1) & 0xFF)
    } else {
        // When done is 0, out_byte is don't-care - assign 0 for clarity
        out_byte = 0
    }
}

func main() {}
