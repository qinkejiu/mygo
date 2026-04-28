package main

var out_byte uint8
var out_done bool

func TopModule(clk bool, in bool, reset bool) {
    // State encoding
    const (
        START = iota
        B0
        B1
        B2
        B3
        B4
        B5
        B6
        B7
        STOP
        DONE
        ERR
    )

    // State register
    var state uint8
    var next uint8
    
    // Shift register for capturing bits (10 bits: start + 8 data + stop)
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
            
            // Shift in new bit at LSB, shift out MSB
            var inBit uint16
            if in {
                inBit = 1
            } else {
                inBit = 0
            }
            byte_r = (byte_r >> 1) | (inBit << 9)
        }
    }

    // Output logic
    out_done = (state == DONE)
    if out_done {
        // Extract bits 8:1 from the shift register (data bits)
        out_byte = uint8((byte_r >> 1) & 0xFF)
    } else {
        // Don't-care when done is not asserted
        out_byte = 0
    }
}

func main() {}
