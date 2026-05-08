package main

var out_out uint8

func TopModule(in uint8) {
    // Reverse bits of 8-bit input
    out_out = ((in & 0x01) << 7) |
        ((in & 0x02) << 5) |
        ((in & 0x04) << 3) |
        ((in & 0x08) << 1) |
        ((in & 0x10) >> 1) |
        ((in & 0x20) >> 3) |
        ((in & 0x40) >> 5) |
        ((in & 0x80) >> 7)
}

func main() {}
