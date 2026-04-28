package main

var out_pos uint8

func TopModule(in uint8) {
    switch in {
    case 0x0:
        out_pos = 0
    case 0x1:
        out_pos = 0
    case 0x2:
        out_pos = 1
    case 0x3:
        out_pos = 0
    case 0x4:
        out_pos = 2
    case 0x5:
        out_pos = 0
    case 0x6:
        out_pos = 1
    case 0x7:
        out_pos = 0
    case 0x8:
        out_pos = 3
    case 0x9:
        out_pos = 0
    case 0xa:
        out_pos = 1
    case 0xb:
        out_pos = 0
    case 0xc:
        out_pos = 2
    case 0xd:
        out_pos = 0
    case 0xe:
        out_pos = 1
    case 0xf:
        out_pos = 0
    default:
        out_pos = 0
    }
}

func main() {}
