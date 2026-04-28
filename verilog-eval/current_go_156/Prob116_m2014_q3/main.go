package main

var out_f bool

func TopModule(x uint8) {
    switch x {
    case 0x0:
        out_f = false // don't-care, choose convenient value
    case 0x1:
        out_f = false // don't-care, choose convenient value
    case 0x2:
        out_f = false
    case 0x3:
        out_f = false // don't-care, choose convenient value
    case 0x4:
        out_f = true
    case 0x5:
        out_f = false // don't-care, choose convenient value
    case 0x6:
        out_f = true
    case 0x7:
        out_f = false
    case 0x8:
        out_f = false
    case 0x9:
        out_f = false
    case 0xa:
        out_f = false // don't-care, choose convenient value
    case 0xb:
        out_f = true
    case 0xc:
        out_f = true
    case 0xd:
        out_f = true // don't-care, choose convenient value
    case 0xe:
        out_f = true
    case 0xf:
        out_f = true // don't-care, choose convenient value
    default:
        out_f = false
    }
}

func main() {}
