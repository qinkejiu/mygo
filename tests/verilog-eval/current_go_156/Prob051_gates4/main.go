package main

var out_and bool
var out_or bool
var out_xor bool

func TopModule(in uint8) {
    // Extract individual bits from the 4-bit input
    b0 := (in & 0x1) != 0
    b1 := (in & 0x2) != 0
    b2 := (in & 0x4) != 0
    b3 := (in & 0x8) != 0

    // 4-input AND gate
    out_and = b0 && b1 && b2 && b3

    // 4-input OR gate
    out_or = b0 || b1 || b2 || b3

    // 4-input XOR gate (parity calculation)
    out_xor = b0 != b1 != b2 != b3
}

func main() {}
