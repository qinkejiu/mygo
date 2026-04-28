package main

var out_q uint8

func TopModule(clk bool, areset bool, d uint8) {
    if areset {
        out_q = 0
    } else if clk {
        out_q = d
    }
}

func main() {}
