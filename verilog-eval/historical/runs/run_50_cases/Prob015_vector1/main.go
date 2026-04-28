package main

var out_hi uint8
var out_lo uint8

func TopModule(in uint16) {
    out_hi = uint8((in >> 8) & 0xFF)
    out_lo = uint8(in & 0xFF)
}

func main() {}
