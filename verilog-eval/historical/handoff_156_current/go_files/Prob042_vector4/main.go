package main

var out_out uint32

func TopModule(in uint8) {
    // Sign-extend 8-bit input to 32-bit output
    // If MSB (bit 7) is 1, fill upper 24 bits with 1s, otherwise with 0s
    if (in & 0x80) != 0 {
        out_out = uint32(in) | 0xFFFFFF00
    } else {
        out_out = uint32(in)
    }
}

func main() {}
