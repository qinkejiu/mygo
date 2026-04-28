package main

var out_out uint32

func TopModule(clk bool, reset bool, in uint32) {
    var d_last uint32
    var out_reg uint32

    if clk {
        if reset {
            out_reg = 0
        } else {
            out_reg = out_reg | (^in & d_last)
        }
        d_last = in
        out_out = out_reg
    }
}

func main() {}
