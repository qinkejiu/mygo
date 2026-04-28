package main

var out_out uint8

func TopModule(in uint8) {
    count := uint8(0)
    if (in & 0x1) != 0 {
        count++
    }
    if (in & 0x2) != 0 {
        count++
    }
    if (in & 0x4) != 0 {
        count++
    }
    out_out = count & 0x3
}

func main() {}
