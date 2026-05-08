package main

var out_bytes uint32
var out_done bool

func TopModule(clk bool, reset bool, in uint8) {
    const (
        BYTE1 = 0
        BYTE2 = 1
        BYTE3 = 2
        DONE  = 3
    )

    var state uint8 = BYTE1
    var next uint8 = BYTE1
    var out_bytes_r uint32 = 0

    in3 := (in & 0x08) != 0

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

    if clk {
        if reset {
            state = BYTE1
            out_bytes_r = 0
        } else {
            state = next
            out_bytes_r = (out_bytes_r << 8) | uint32(in)
        }
    }

    out_done = (state == DONE)

    if out_done {
        out_bytes = out_bytes_r
    } else {
        out_bytes = 0
    }
}

func main() {}
