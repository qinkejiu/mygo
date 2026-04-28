package main

var out_out uint8
var out_valid bool

func TopModule(code uint8) {
    out_out = 0
    out_valid = false

    switch code {
    case 0x45:
        out_out = 0
        out_valid = true
    case 0x16:
        out_out = 1
        out_valid = true
    case 0x1e:
        out_out = 2
        out_valid = true
    case 0x26:
        out_out = 3
        out_valid = true
    case 0x25:
        out_out = 4
        out_valid = true
    case 0x2e:
        out_out = 5
        out_valid = true
    case 0x36:
        out_out = 6
        out_valid = true
    case 0x3d:
        out_out = 7
        out_valid = true
    case 0x3e:
        out_out = 8
        out_valid = true
    case 0x46:
        out_out = 9
        out_valid = true
    default:
        out_out = 0
        out_valid = false
    }
}

func main() {}
