package main

var out_q uint16

func TopModule(a uint8) {
    switch a & 0x7 {
    case 0:
        out_q = 4658
    case 1:
        out_q = 44768
    case 2:
        out_q = 10196
    case 3:
        out_q = 23054
    case 4:
        out_q = 8294
    case 5:
        out_q = 25806
    case 6:
        out_q = 50470
    case 7:
        out_q = 12057
    default:
        out_q = 0
    }
}

func main() {}
