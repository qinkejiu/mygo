package main

var out_mux_in uint8

func TopModule(c bool, d bool) {
    // mux_in[0] = c | d
    mux_in0 := c || d
    
    // mux_in[1] = 0
    mux_in1 := false
    
    // mux_in[2] = ~d
    mux_in2 := !d
    
    // mux_in[3] = c & d
    mux_in3 := c && d
    
    // Pack the 4 bits into a uint8
    out_mux_in = 0
    if mux_in0 {
        out_mux_in |= 0x1
    }
    if mux_in1 {
        out_mux_in |= 0x2
    }
    if mux_in2 {
        out_mux_in |= 0x4
    }
    if mux_in3 {
        out_mux_in |= 0x8
    }
}

func main() {}
