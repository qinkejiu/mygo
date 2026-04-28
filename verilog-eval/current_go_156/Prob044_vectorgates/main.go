package main

var out_or_bitwise uint8
var out_or_logical bool
var out_not uint8

func TopModule(a uint8, b uint8) {
    // Bitwise OR of two 3-bit inputs
    out_or_bitwise = (a | b) & 0x07
    
    // Logical OR: true if any bit in a or b is non-zero
    out_or_logical = (a & 0x07) != 0 || (b & 0x07) != 0
    
    // NOT of both vectors: ~b in upper 3 bits, ~a in lower 3 bits
    not_a := (^a) & 0x07
    not_b := (^b) & 0x07
    out_not = (not_b << 3) | not_a
}

func main() {}
