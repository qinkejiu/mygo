package main

var out_Y1 bool

func TopModule(y uint8, w bool) {
    // Extract individual bits from y (3-bit input)
    // y[0] is LSB, y[1] is middle bit, y[2] is MSB
    y0 := (y & 0x1) != 0
    y1 := (y & 0x2) != 0
    y2 := (y & 0x4) != 0
    
    // Output Y1 is simply y[1] (current state bit)
    out_Y1 = y1
}

func main() {}
