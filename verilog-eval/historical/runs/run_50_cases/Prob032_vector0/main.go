package main

var out_outv uint8
var out_o2 bool
var out_o1 bool
var out_o0 bool

func TopModule(vec uint8) {
    // Output the same 3-bit vector
    out_outv = vec & 0x07
    
    // Split into individual bits
    out_o0 = (vec & 0x01) != 0
    out_o1 = (vec & 0x02) != 0
    out_o2 = (vec & 0x04) != 0
}

func main() {}
