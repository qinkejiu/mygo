package main

var out_both uint8
var out_any uint8
var out_different uint8

func TopModule(in uint8) {
    // out_both[2:0] = in[2:0] & in[3:1]
    // We'll compute using bitwise operations on the 4-bit input
    // Extract relevant bits and shift to align
    in_low := in & 0x07        // in[2:0]
    in_high_shifted := (in >> 1) & 0x07  // in[3:1]
    out_both_low := in_low & in_high_shifted
    
    // Combine with don't-care for bit 3
    out_both = (out_both_low & 0x07) | ((in & 0x08) & 0x00)  // bit 3 forced to 0
    
    // out_any[3:1] = in[2:0] | in[3:1]
    out_any_high := in_low | in_high_shifted
    
    // Combine with don't-care for bit 0
    out_any = ((out_any_high & 0x07) << 1) | (in & 0x01 & 0x00)  // bit 0 forced to 0
    
    // out_different = in ^ {in[0], in[3:1]}
    // Create rotated version: {in[0], in[3:1]}
    rotated := ((in & 0x01) << 3) | ((in >> 1) & 0x07)
    out_different = in ^ rotated
}

func main() {}
