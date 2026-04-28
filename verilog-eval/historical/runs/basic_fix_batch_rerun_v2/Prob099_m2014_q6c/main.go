package main

var out_Y1 bool
var out_Y3 bool

func TopModule(y uint8, w bool) {
    // Extract one-hot state bits from y[5:0]
    // y[0] = bit 0 (state A)
    // y[1] = bit 1 (state B)
    // y[2] = bit 2 (state C)
    // y[3] = bit 3 (state D)
    // y[4] = bit 4 (state E)
    // y[5] = bit 5 (state F)
    
    // Y1 = y[0] & ~w
    out_Y1 = (y & 0x01) != 0 && !w
    
    // Y3 = (y[1] | y[2] | y[4] | y[5]) & w
    y1_or_y2_or_y4_or_y5 := (y & 0x02) != 0 || // y[1]
                            (y & 0x04) != 0 || // y[2]
                            (y & 0x10) != 0 || // y[4]
                            (y & 0x20) != 0    // y[5]
    out_Y3 = y1_or_y2_or_y4_or_y5 && w
}

func main() {}
