package main

var out_and bool
var out_or bool
var out_xor bool

func TopModule(in uint8) {
    // Extract individual bits from the 4-bit input
    in0 := (in & 0x1) != 0
    in1 := (in & 0x2) != 0
    in2 := (in & 0x4) != 0
    in3 := (in & 0x8) != 0

    // 4-input AND gate
    out_and = in0 && in1 && in2 && in3

    // 4-input OR gate
    out_or = in0 || in1 || in2 || in3

    // 4-input XOR gate (parity calculation)
    out_xor = in0 != in1 != in2 != in3
}

func main() {}
