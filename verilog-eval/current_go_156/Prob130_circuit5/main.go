package main

var out_q uint8

func TopModule(a uint8, b uint8, c uint8, d uint8, e uint8) {
    switch c {
    case 0:
        out_q = b
    case 1:
        out_q = e
    case 2:
        out_q = a
    case 3:
        out_q = d
    default:
        out_q = 0xF
    }
}

func main() {}
