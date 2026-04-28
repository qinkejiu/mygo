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
    staticState := BYTE1
    staticOutBytes := uint32(0)

    // Next state logic
    nextState := staticState
    switch staticState {
    case BYTE1:
        if (in & 0x08) != 0 { // Check in[3] = 1
            nextState = BYTE2
        } else {
            nextState = BYTE1
        }
    case BYTE2:
        nextState = BYTE3
    case BYTE3:
        nextState = DONE
    case DONE:
        if (in & 0x08) != 0 { // Check in[3] = 1
            nextState = BYTE2
        } else {
            nextState = BYTE1
        }
    }

    // Shift register for collecting bytes
    nextOutBytes := staticOutBytes
    if staticState == BYTE1 || staticState == BYTE2 || staticState == BYTE3 {
        nextOutBytes = (staticOutBytes << 8) | uint32(in)
    }

    // Clocked behavior
    if clk {
        if reset {
            staticState = BYTE1
            staticOutBytes = 0
        } else {
            staticState = nextState
            staticOutBytes = nextOutBytes
        }
    }

    // Output logic
    out_done = (staticState == DONE)
    if out_done {
        out_bytes = staticOutBytes
    } else {
        // Output don't-care when not done
        out_bytes = 0
    }
}

func main() {}
