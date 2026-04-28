package main

var out_both uint8
var out_any uint8
var out_different uint8

func TopModule(in uint8) {
    // Extract individual bits for clarity
    in0 := (in & 0x1) != 0
    in1 := (in & 0x2) != 0
    in2 := (in & 0x4) != 0
    in3 := (in & 0x8) != 0

    // out_both[2:0] = in[2:0] & in[3:1]
    // We don't care about out_both[3]
    out_both = 0
    if in2 && in3 {
        out_both |= 0x4 // bit 2
    }
    if in1 && in2 {
        out_both |= 0x2 // bit 1
    }
    if in0 && in1 {
        out_both |= 0x1 // bit 0
    }

    // out_any[3:1] = in[2:0] | in[3:1]
    // We don't care about out_any[0]
    out_any = 0
    if in3 || in2 {
        out_any |= 0x8 // bit 3
    }
    if in2 || in1 {
        out_any |= 0x4 // bit 2
    }
    if in1 || in0 {
        out_any |= 0x2 // bit 1
    }

    // out_different = in ^ {in[0], in[3:1]}
    // This is XOR between in and {in[0], in[3], in[2], in[1]}
    shifted := uint8(0)
    if in0 {
        shifted |= 0x8 // in[0] becomes bit 3
    }
    if in3 {
        shifted |= 0x4 // in[3] becomes bit 2
    }
    if in2 {
        shifted |= 0x2 // in[2] becomes bit 1
    }
    if in1 {
        shifted |= 0x1 // in[1] becomes bit 0
    }
    out_different = in ^ shifted
}

func main() {}
