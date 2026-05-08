package main

var out_done bool

func TopModule(clk bool, reset bool, in uint8) {
    // State encoding
    const (
        BYTE1 = 0
        BYTE2 = 1
        BYTE3 = 2
        DONE  = 3
    )

    // State register (2 bits)
    var state uint8
    var next uint8

    // Extract in[3]
    in3 := (in & 0x08) != 0

    // Combinational next state logic
    switch state {
    case BYTE1:
        if in3 {
            next = BYTE2
        } else {
            next = BYTE1
        }
    case BYTE2:
        next = BYTE3
    case BYTE3:
        next = DONE
    case DONE:
        if in3 {
            next = BYTE2
        } else {
            next = BYTE1
        }
    }

    // Sequential logic (positive edge triggered)
    if clk {
        if reset {
            state = BYTE1
        } else {
            state = next
        }
    }

    // Output logic
    out_done = (state == DONE)
}

func main() {}
