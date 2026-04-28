package main

var out_out bool

func TopModule(in [256]bool, sel uint8) {
    out_out = in[sel]
}

func main() {}
