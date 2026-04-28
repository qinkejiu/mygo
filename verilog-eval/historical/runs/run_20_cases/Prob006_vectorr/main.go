package main

var out_out [8]bool

func TopModule(in [8]bool) {
    out_out[0] = in[7]
    out_out[1] = in[6]
    out_out[2] = in[5]
    out_out[3] = in[4]
    out_out[4] = in[3]
    out_out[5] = in[2]
    out_out[6] = in[1]
    out_out[7] = in[0]
}

func main() {}
