package main

var out_out uint32

func TopModule(in uint32) {
    out_out = ((in & 0x000000FF) << 24) |
        ((in & 0x0000FF00) << 8) |
        ((in & 0x00FF0000) >> 8) |
        ((in & 0xFF000000) >> 24)
}

func main() {}
