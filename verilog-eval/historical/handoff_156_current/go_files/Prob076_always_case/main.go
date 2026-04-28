package main

var out_out uint8

func TopModule(sel uint8, data0 uint8, data1 uint8, data2 uint8, data3 uint8, data4 uint8, data5 uint8) {
    switch sel {
    case 0:
        out_out = data0 & 0xF
    case 1:
        out_out = data1 & 0xF
    case 2:
        out_out = data2 & 0xF
    case 3:
        out_out = data3 & 0xF
    case 4:
        out_out = data4 & 0xF
    case 5:
        out_out = data5 & 0xF
    default:
        out_out = 0
    }
}

func main() {}
