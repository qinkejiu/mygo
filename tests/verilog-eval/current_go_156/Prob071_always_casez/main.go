package main

var out_pos uint8

func TopModule(in uint8) {
    // Priority encoder: find first 1 from LSB (bit 0) to MSB (bit 7)
    // Output 0 if no bits are set
    switch {
    case (in & 0x01) != 0:
        out_pos = 0
    case (in & 0x02) != 0:
        out_pos = 1
    case (in & 0x04) != 0:
        out_pos = 2
    case (in & 0x08) != 0:
        out_pos = 3
    case (in & 0x10) != 0:
        out_pos = 4
    case (in & 0x20) != 0:
        out_pos = 5
    case (in & 0x40) != 0:
        out_pos = 6
    case (in & 0x80) != 0:
        out_pos = 7
    default:
        out_pos = 0
    }
}

func main() {}
