package main

var out_out bool

func TopModule(in [256]bool, sel uint8) {
    // Use sel as an index into the 256-bit array
    // Since in[0] is LSB, we can directly index with sel
    out_out = in[sel]
}

func main() {}
