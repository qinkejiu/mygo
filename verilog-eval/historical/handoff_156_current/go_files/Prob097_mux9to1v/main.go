package main

var out_out uint16

func TopModule(a uint16, b uint16, c uint16, d uint16, e uint16, f uint16, g uint16, h uint16, i uint16, sel uint8) {
    out_out = 0xFFFF // default to all ones for sel=9..15

    switch sel {
    case 0x0:
        out_out = a
    case 0x1:
        out_out = b
    case 0x2:
        out_out = c
    case 0x3:
        out_out = d
    case 0x4:
        out_out = e
    case 0x5:
        out_out = f
    case 0x6:
        out_out = g
    case 0x7:
        out_out = h
    case 0x8:
        out_out = i
    }
}

func main() {}
