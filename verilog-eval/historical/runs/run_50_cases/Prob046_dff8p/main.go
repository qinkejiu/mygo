package main

var out_q uint8

func TopModule(clk bool, reset bool, d uint8) {
    if !clk { // negative edge triggered
        if reset {
            out_q = 0x34
        } else {
            out_q = d
        }
    }
}

func main() {}
