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

    // State register (4 bits to hold values 0-11)
    var state uint8
    var next uint8

    // Shift register for capturing bits (10 bits total: stop + 8 data + start)
    var byte_r uint16

    // Combinational next state logic
    switch state {
    case START:
        if in {
            next = START
        } else {
            next = B0
        }
    case B0:
        next = B1
    case B1:
        next = B2
    case B2:
        next = B3
    case B3:
        next = B4
    case B4:
        next = B5
    case B5:
        next = B6
    case B6:
        next = B7
    case B7:
        next = STOP
    case STOP:
        if in {
            next = DONE
        } else {
            next = ERR
        }
    case DONE:
        if in {
            next = START
        } else {
            next = B0
        }
    case ERR:
        if in {
            next = START
        } else {
            next = ERR
        }
    default:
        next = START
    }

    // Sequential logic (positive edge triggered)
    if clk {
        if reset {
            state = START
            byte_r = 0
        } else {
            state = next
            
            // Shift register update
            byte_r = (byte_r >> 1) | (uint16(in) << 9)
        }
    }

    // Output logic
    out_done = (state == DONE)
    if out_done {
        // Extract bits 8:1 from byte_r (LSB first was shifted in)
        out_byte = uint8((byte_r >> 1) & 0xFF)
    } else {
        out_byte = 0 // Don't-care when not done
    }
}

func main() {}
