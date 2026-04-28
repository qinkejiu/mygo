package main

var out_s uint8
var out_overflow bool

func TopModule(a uint8, b uint8) {
    sum := uint16(a) + uint16(b)
    out_s = uint8(sum & 0xFF)
    
    a_msb := (a >> 7) & 1
    b_msb := (b >> 7) & 1
    s_msb := (out_s >> 7) & 1
    
    out_overflow = (a_msb == b_msb) && (a_msb != s_msb)
}

func main() {}
