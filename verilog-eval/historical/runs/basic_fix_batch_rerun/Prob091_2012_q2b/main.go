package main

var out_Y1 bool
var out_Y3 bool

func TopModule(y uint8, w bool) {
    // Extract individual state bits from y[5:0]
    // y[0] is LSB (state A), y[5] is MSB (state F)
    y0 := (y & 0x01) != 0
    y1 := (y & 0x02) != 0
    y2 := (y & 0x04) != 0
    y4 := (y & 0x10) != 0
    y5 := (y & 0x20) != 0

    // Y1 = y[0] & w
    out_Y1 = y0 && w

    // Y3 = (y[1] | y[2] | y[4] | y[5]) & ~w
    out_Y3 = (y1 || y2 || y4 || y5) && !w
}

func main() {}
