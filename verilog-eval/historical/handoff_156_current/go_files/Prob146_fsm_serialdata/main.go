package main

var out_byte uint8
var out_done bool

func TopModule(clk bool, in bool, reset bool) {
    // State encoding
    const (
        STATE_START = iota
        STATE_B0
        STATE_B1
        STATE_B2
        STATE_B3
        STATE_B4
        STATE_B5
        STATE_B6
        STATE_B7
        STATE_STOP
        STATE_DONE
        STATE_ERR
    )

    // State register
    var state uint8
    var next_state uint8
    
    // Shift register for capturing bits (10 bits total: start + 8 data + stop)
    var shift_reg uint16

    // Sequential logic
    if clk {
        if reset {
            state = STATE_START
            shift_reg = 0
        } else {
            state = next_state
            // Shift in new bit at LSB
            shift_reg = (shift_reg >> 1)
            if in {
                shift_reg |= 0x200 // Set bit 9 (10th bit) when in is 1
            }
        }
    }

    // Combinational next state logic
    switch state {
    case STATE_START:
        if in {
            next_state = STATE_START
        } else {
            next_state = STATE_B0
        }
    case STATE_B0:
        next_state = STATE_B1
    case STATE_B1:
        next_state = STATE_B2
    case STATE_B2:
        next_state = STATE_B3
    case STATE_B3:
        next_state = STATE_B4
    case STATE_B4:
        next_state = STATE_B5
    case STATE_B5:
        next_state = STATE_B6
    case STATE_B6:
        next_state = STATE_B7
    case STATE_B7:
        next_state = STATE_STOP
    case STATE_STOP:
        if in {
            next_state = STATE_DONE
        } else {
            next_state = STATE_ERR
        }
    case STATE_DONE:
        if in {
            next_state = STATE_START
        } else {
            next_state = STATE_B0
        }
    case STATE_ERR:
        if in {
            next_state = STATE_START
        } else {
            next_state = STATE_ERR
        }
    default:
        next_state = STATE_START
    }

    // Output logic
    out_done = (state == STATE_DONE)
    if out_done {
        // Extract bits 8:1 from shift_reg (bits 8 down to 1)
        // shift_reg[8:1] corresponds to the 8 data bits
        out_byte = uint8((shift_reg >> 1) & 0xFF)
    } else {
        out_byte = 0 // Don't-care when done is 0
    }
}

func main() {}
